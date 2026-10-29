package tools

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/folders"
)

func TestParseFolderID(t *testing.T) {
	tests := []struct {
		name  string
		raw   string
		valid bool
		ok    bool
	}{
		{"absent means the default folder", "", false, true},
		{"a UUID", "6f1c2f0e-8c1d-4a53-9f7a-0e5a8f3f2b11", true, true},
		{"not a UUID", "checkout", false, false},
		{"a title is not an id", "General", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, res, ok := parseFolderID(tt.raw)
			if ok != tt.ok || id.Valid != tt.valid {
				t.Fatalf("parseFolderID(%q) = valid %v, ok %v; want valid %v, ok %v", tt.raw, id.Valid, ok, tt.valid, tt.ok)
			}
			if !ok && errorCode(t, res) != codeInvalidRequest {
				t.Fatalf("res = %#v", res)
			}
		})
	}
}

func TestCreateFolderFailure(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		code    string
		message string
		mapped  bool
	}{
		{"folder gone", dashboards.ErrFolderNotFound, codeNotFound, "folder not found", true},
		{"folder gone inside a wrapped error", fmt.Errorf("save: %w", dashboards.ErrFolderNotFound), codeNotFound, "folder not found", true},
		{"role cannot create there", folders.ErrForbidden, codeForbiddenRole, "you cannot create dashboards in this folder", true},
		{"a dashboard that does not exist is not a folder problem", dashboards.ErrNotFound, "", "", false},
		{"an unrelated error", errors.New("boom"), "", "", false},
		{"no error", nil, "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, mapped := createFolderFailure(tt.err)
			if mapped != tt.mapped {
				t.Fatalf("mapped = %v, want %v", mapped, tt.mapped)
			}
			if !mapped {
				return
			}
			body := res.Content.(errorBody)
			if errorCode(t, res) != tt.code || body.Message != tt.message {
				t.Fatalf("res = %#v", res.Content)
			}
		})
	}
}

func TestFolderInputsAreCheckedBeforeAnythingIsSaved(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	longTitle := strings.Repeat("x", 101)
	tests := []struct {
		name  string
		tool  string
		input string
	}{
		{"create_dashboard with a folder id that is not a UUID", nameCreateDashboard, `{"spec":{},"folder_id":"checkout"}`},
		{"import with an empty folder", nameImportGrafana, `{"json":"{}","folder":""}`},
		{"import with a blank folder", nameImportGrafana, `{"json":"{}","folder":"   "}`},
		{"import with a folder title over 100 characters", nameImportGrafana, `{"json":"{}","folder":"` + longTitle + `"}`},
		{"import with a folder that is not a string", nameImportGrafana, `{"json":"{}","folder":5}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := call(t, r, tt.tool, tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if errorCode(t, res) != codeInvalidRequest {
				t.Fatalf("res = %#v", res.Content)
			}
		})
	}
}
