package folders

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

const CodeRestrictedContent = "restricted_content"

func AdminsOnly(f dbsqlc.Folder) bool {
	return f.AdminsOnly
}

type ContentKind string

const (
	ContentDashboard   ContentKind = "dashboard"
	ContentObservation ContentKind = "observation"
	ContentSavedSearch ContentKind = "saved_search"
	ContentProposal    ContentKind = "proposal"
)

func (k ContentKind) noun() string {
	switch k {
	case ContentObservation:
		return "observation"
	case ContentSavedSearch:
		return "saved search"
	case ContentProposal:
		return "proposal"
	}
	return "dashboard"
}

type ContentItem struct {
	Kind        ContentKind
	ID          string
	Title       string
	FolderID    string
	FolderTitle string
}

type RestrictedContentError struct {
	Datasource string
	Items      []ContentItem
	message    string
}

func (e *RestrictedContentError) Error() string { return e.message }

func RestrictedSave(kind ContentKind, datasource string) error {
	return &RestrictedContentError{Datasource: datasource, message: fmt.Sprintf(
		"this %s reads the datasource %q, which only Admins can query; save it in a folder only Admins can read", kind.noun(), datasource)}
}

func RestrictedMove(kind ContentKind, datasource string) error {
	return &RestrictedContentError{Datasource: datasource, message: fmt.Sprintf(
		"this %s reads the datasource %q, which only Admins can query, so it can only move to another folder only Admins can read", kind.noun(), datasource)}
}

func RestrictedOpening(item ContentItem, datasource string) error {
	return &RestrictedContentError{Datasource: datasource, Items: []ContentItem{item}, message: fmt.Sprintf(
		"the %s %q in this folder reads the datasource %q, which only Admins can query; move it to another folder only Admins can read before you share this one", item.Kind.noun(), item.Title, datasource)}
}

func RestrictedAudience(datasource string, items []ContentItem) error {
	return &RestrictedContentError{Datasource: datasource, Items: items, message: fmt.Sprintf(
		"dashboards, observations or saved searches outside folders only Admins can read use the datasource %q; move them to a folder only Admins can read before you make it Admins only", datasource)}
}

type contentItemJSON struct {
	Kind        ContentKind `json:"kind"`
	ID          string      `json:"id"`
	Title       string      `json:"title"`
	FolderID    string      `json:"folder_id"`
	FolderTitle string      `json:"folder_title"`
}

func AbortRestricted(c *gin.Context, err error) bool {
	var restricted *RestrictedContentError
	if !errors.As(err, &restricted) {
		return false
	}
	body := gin.H{"error": restricted.Error(), "code": CodeRestrictedContent, "datasource": restricted.Datasource}
	if len(restricted.Items) > 0 {
		items := make([]contentItemJSON, len(restricted.Items))
		for i, it := range restricted.Items {
			items[i] = contentItemJSON(it)
		}
		body["items"] = items
	}
	c.AbortWithStatusJSON(http.StatusConflict, body)
	return true
}
