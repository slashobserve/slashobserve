package forbidden

import (
	"errors"
	"strings"
)

const (
	Code          = "forbidden_role"
	RoleMessage   = "your role does not allow this"
	AccessMessage = "you do not have access to this dashboard"
)

type Body struct {
	Error        string `json:"error"`
	Code         string `json:"code"`
	NeededRole   string `json:"needed_role,omitempty"`
	NeededAccess string `json:"needed_access,omitempty"`
	FolderTitle  string `json:"folder_title,omitempty"`
}

func NeedsRole(role string) Body {
	return Body{Error: RoleMessage, Code: Code, NeededRole: role}
}

func NeedsAccess(level, folderTitle string) Body {
	return Body{Error: RoleMessage, Code: Code, NeededAccess: level, FolderTitle: folderTitle}
}

func (b Body) Message() string {
	switch {
	case b.NeededAccess != "" && b.FolderTitle != "":
		return b.Error + "; it needs " + capitalized(b.NeededAccess) + " on " + b.FolderTitle
	case b.NeededAccess != "":
		return b.Error + "; it needs " + capitalized(b.NeededAccess) + " on the folder"
	case b.NeededRole != "":
		return b.Error + "; it needs the " + capitalized(b.NeededRole) + " role"
	}
	return b.Error
}

func capitalized(word string) string {
	return strings.ToUpper(word[:1]) + word[1:]
}

type needed struct {
	err  error
	body Body
}

func (n *needed) Error() string { return n.err.Error() }

func (n *needed) Unwrap() error { return n.err }

func Need(err error, body Body) error { return &needed{err: err, body: body} }

func BodyOf(err error) Body {
	var n *needed
	if errors.As(err, &n) {
		return n.body
	}
	return Body{Error: RoleMessage, Code: Code}
}
