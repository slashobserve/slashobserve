package datasourceadmin

import (
	"errors"
	"testing"
)

func TestUpdateAndTestRefuseAnAddressChangeThatWouldCarryStoredCredentials(t *testing.T) {
	var seen string
	server := upstream(t, &seen)
	r, svc := newRig(t, testKey)
	if _, err := svc.Create(reading(t.Context()), r.admin, prometheus("added", server.URL), Secrets{BearerToken: "token-1"}); err != nil {
		t.Fatal(err)
	}
	moved := prometheus("added", "http://other.example.com:9090")

	if _, err := svc.Update(reading(t.Context()), r.admin, "added", moved, SecretsChange{}); !errors.Is(err, ErrSecretRequired) {
		t.Fatalf("update err = %v, want ErrSecretRequired", err)
	}
	if _, err := svc.Test(reading(t.Context()), "added", moved, SecretsChange{}); !errors.Is(err, ErrSecretRequired) {
		t.Fatalf("test err = %v, want ErrSecretRequired", err)
	}
	record, err := svc.Get(reading(t.Context()), "added")
	if err != nil || record.Definition.Address != server.URL {
		t.Fatalf("record = %+v, err = %v", record, err)
	}
	if _, err := svc.Update(reading(t.Context()), r.admin, "added", moved, SecretsChange{Set: Secrets{BearerToken: "token-2"}}); err != nil {
		t.Fatalf("update with the credential again: %v", err)
	}
}
