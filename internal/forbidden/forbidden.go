package forbidden

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
