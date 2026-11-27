package datasourceadmin

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func storedRow(t *testing.T, def Definition, secrets Secrets) dbsqlc.Datasource {
	t.Helper()
	id := newID()
	config, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	sealedSecrets, err := sealSecrets(testKey, id, secrets)
	if err != nil {
		t.Fatal(err)
	}
	return dbsqlc.Datasource{ID: id, Uid: def.UID, Type: def.Type, Name: def.Name, Config: config, Secrets: sealedSecrets, SecretNames: namesOrEmpty(secrets)}
}

func TestComposeRefusesToCarryStoredCredentialsToAChangedAddressOrLogin(t *testing.T) {
	svc := &Service{key: testKey}
	stored := prometheus("prom", "http://prom.example.com:9090")
	stored.Username = "ada"
	secrets := Secrets{Password: "pw", BearerToken: "token", Headers: map[string]string{"X-Org": "7", "X-Team": "a"}}
	row := storedRow(t, stored, secrets)

	moved := stored
	moved.Address = "http://attacker.example.com:9090"
	relogin := stored
	relogin.Username = "grace"
	spaced := stored
	spaced.Address = "  http://prom.example.com:9090 "
	renamed := stored
	renamed.Name = "Renamed"

	cases := []struct {
		name    string
		def     Definition
		change  SecretsChange
		wantErr bool
	}{
		{"address change with no secret", moved, SecretsChange{}, true},
		{"login change with no secret", relogin, SecretsChange{}, true},
		{"address change that sets only one of several", moved, SecretsChange{Set: Secrets{Password: "new"}}, true},
		{"address change that leaves one header", moved, SecretsChange{Set: Secrets{Password: "new", BearerToken: "t"}, Clear: []string{"header:X-Org"}}, true},
		{"address change that resends every credential", moved, SecretsChange{Set: Secrets{Password: "new", BearerToken: "t", Headers: map[string]string{"X-Org": "1", "X-Team": "b"}}}, false},
		{"address change that clears every credential", moved, SecretsChange{Clear: []string{"password", "bearer_token", "header:X-Org", "header:X-Team"}}, false},
		{"address with stray spaces is the same address", spaced, SecretsChange{}, false},
		{"rename keeps the stored credentials", renamed, SecretsChange{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := svc.compose(row, tc.def, tc.change)
			if tc.wantErr {
				if !errors.Is(err, ErrSecretRequired) {
					t.Fatalf("err = %v, want ErrSecretRequired", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.change.empty() && got.Password != "pw" {
				t.Fatalf("stored password was not kept: %+v", got)
			}
		})
	}
}

func TestComposeLetsAnAddressChangeThroughWhenNothingIsStored(t *testing.T) {
	svc := &Service{key: testKey}
	stored := prometheus("prom", "http://prom.example.com:9090")
	row := storedRow(t, stored, Secrets{})
	moved := stored
	moved.Address = "http://other.example.com:9090"
	if _, err := svc.compose(row, moved, SecretsChange{}); err != nil {
		t.Fatalf("err = %v, want none", err)
	}
}

func TestFailMapsTheCredentialCarryAndTheLostRaceToDocumentedCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(nil, slog.New(slog.DiscardHandler))
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{ErrSecretRequired, http.StatusUnprocessableEntity, "secret_required"},
		{ErrChanged, http.StatusConflict, "datasource_changed"},
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPatch, "/api/v1/datasources/prom", nil)
		h.fail(c, "change datasource", tc.err)
		if w.Code != tc.status || !strings.Contains(w.Body.String(), `"code":"`+tc.code+`"`) {
			t.Fatalf("%v answered %d %s", tc.err, w.Code, w.Body)
		}
	}
}
