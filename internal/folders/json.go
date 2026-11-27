package folders

import (
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

type Permissions struct {
	Comment bool `json:"comment"`
	Edit    bool `json:"edit"`
	Manage  bool `json:"manage"`
	Create  bool `json:"create"`
}

func PermissionsJSON(a Abilities) Permissions {
	return Permissions{Comment: a.Comment, Edit: a.Edit, Manage: a.Manage, Create: a.Create}
}

type creatorJSON struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

type folderJSON struct {
	ID             string       `json:"id"`
	Title          string       `json:"title"`
	General        bool         `json:"general"`
	Personal       bool         `json:"personal"`
	Restricted     bool         `json:"restricted"`
	AdminsOnly     bool         `json:"admins_only"`
	Audit          bool         `json:"audit"`
	Kind           Kind         `json:"kind"`
	AllowedActions []Action     `json:"allowed_actions"`
	DashboardCount int          `json:"dashboard_count"`
	Grantees       []string     `json:"grantees"`
	GranteeCount   int          `json:"grantee_count"`
	CreatedBy      *creatorJSON `json:"created_by"`
	Permissions    Permissions  `json:"permissions"`
	Level          string       `json:"level"`
}

func folderBody(f Folder, a Abilities, d dbsqlc.ListFolderDetailsRow, audit pgtype.UUID) folderJSON {
	isAudit := audit.Valid && audit == f.ID
	body := folderJSON{
		ID:             f.ID.String(),
		Title:          f.Title,
		General:        f.General,
		Personal:       f.Personal,
		Restricted:     f.Restricted,
		AdminsOnly:     f.AdminsOnly,
		Audit:          isAudit,
		Kind:           KindOf(f, isAudit),
		AllowedActions: AllowedActions(f, a),
		DashboardCount: int(d.DashboardCount),
		Grantees:       d.Grantees,
		GranteeCount:   int(d.GranteeCount),
		Permissions:    PermissionsJSON(a),
		Level:          a.Level().String(),
	}
	if body.Grantees == nil {
		body.Grantees = []string{}
	}
	if d.CreatedByKind.Valid && d.CreatedByID.Valid {
		body.CreatedBy = &creatorJSON{Kind: d.CreatedByKind.String, ID: d.CreatedByID.String(), Name: d.CreatedByName.String}
	}
	return body
}

type grantedUserJSON struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

type permissionEntryJSON struct {
	Kind        string           `json:"kind"`
	User        *grantedUserJSON `json:"user,omitempty"`
	Group       *string          `json:"group,omitempty"`
	MemberCount *int             `json:"member_count,omitempty"`
	Level       string           `json:"level"`
}

type folderPermissionsJSON struct {
	Restricted bool                  `json:"restricted"`
	AdminsOnly bool                  `json:"admins_only"`
	Entries    []permissionEntryJSON `json:"entries"`
}

func permissionsBody(folder dbsqlc.Folder, rows []dbsqlc.ListFolderPermissionsRow) folderPermissionsJSON {
	adminsOnly := AdminsOnly(folder)
	body := folderPermissionsJSON{Restricted: Restricted(adminsOnly, len(rows) > 0), AdminsOnly: adminsOnly, Entries: make([]permissionEntryJSON, 0, len(rows))}
	for _, r := range rows {
		e := permissionEntryJSON{Level: r.Level}
		if r.GroupName.Valid {
			group, members := r.GroupName.String, int(r.MemberCount)
			e.Kind, e.Group, e.MemberCount = "group", &group, &members
		} else {
			e.Kind = "user"
			e.User = &grantedUserJSON{ID: r.UserID.String(), Name: r.UserName, Email: r.UserEmail, Role: r.UserRole}
		}
		body.Entries = append(body.Entries, e)
	}
	return body
}
