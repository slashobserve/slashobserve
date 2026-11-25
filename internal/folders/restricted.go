package folders

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/httpapi"
)

const (
	CodeRestrictedContent = "restricted_content"
	CodeAdminsOnlyFolder  = "admins_only_folder"
	CodeRestrictedAnswer  = "restricted_answer"
	CodeProvisioned       = "provisioned"
)

var (
	ErrLinkedMove            = errors.New("this dashboard has public links, and a folder only Admins can read has none; revoke its public links before you move it there")
	ErrLinkedObservationMove = errors.New("an RCA of this observation has public links, and a folder only Admins can read has none; revoke them before you move it there")
	ErrLinkedFolder          = errors.New("a dashboard or an RCA in this folder has public links, and a folder only Admins can read has none; revoke them before you make it Admins only")
	ErrRestrictedAnswer      = errors.New("this answer read data only Admins can read, such as the audit trail or what is in a folder only Admins can read, and people who are not Admins read where it would go; " +
		"keep it in the private chat, or save it in a folder only Admins can read")
)

type restrictedAnswerError struct{ message string }

func (e restrictedAnswerError) Error() string { return e.message }

func (restrictedAnswerError) Is(target error) bool { return target == ErrRestrictedAnswer }

type AnswerRef struct {
	RunID  string
	Prompt string
}

const answerPromptRunes = 80

func (a AnswerRef) described() string {
	if a.RunID == "" {
		return "an answer"
	}
	prompt := strings.Join(strings.Fields(a.Prompt), " ")
	if runes := []rune(prompt); len(runes) > answerPromptRunes {
		prompt = string(runes[:answerPromptRunes-1]) + "…"
	}
	return fmt.Sprintf("the answer to %q (run %s)", prompt, a.RunID)
}

func RestrictedAnswerMove(kind ContentKind, answer AnswerRef) error {
	return restrictedAnswerError{fmt.Sprintf(
		"this %s holds %s that read data only Admins can query, such as the audit trail, so it can only move to another folder only Admins can read", kind.Noun(), answer.described())}
}

func RestrictedAnswerCopy(kind ContentKind) error {
	return restrictedAnswerError{fmt.Sprintf(
		"this %s holds an answer that read data only Admins can query, such as the audit trail, so a copy of it can only go in a folder only Admins can read", kind.Noun())}
}

func RestrictedAnswerOpening(items []ContentItem) error {
	first := items[0]
	message := fmt.Sprintf("the %s %q in this folder holds an answer that read data only Admins can query, such as the audit trail; move it to another folder only Admins can read before you share this one", first.Kind.Noun(), first.Title)
	if len(items) > 1 {
		message = fmt.Sprintf("%d items in this folder, such as the %s %q, hold answers that read data only Admins can query, such as the audit trail; move them to another folder only Admins can read before you share this one", len(items), first.Kind.Noun(), first.Title)
	}
	return restrictedAnswerError{message}
}

func RunInFlightMove(kind ContentKind) error {
	return restrictedAnswerError{fmt.Sprintf(
		"an agent run on this %s has not finished, and what it writes may read data only Admins can query, such as the audit trail; wait for it to end before you move the %s out of folders only Admins can read", kind.Noun(), kind.Noun())}
}

func RunInFlightOpening(items []ContentItem) error {
	first := items[0]
	message := fmt.Sprintf("an agent run on the %s %q in this folder has not finished, and what it writes may read data only Admins can query, such as the audit trail; wait for it to end before you share this folder", first.Kind.Noun(), first.Title)
	if len(items) > 1 {
		message = fmt.Sprintf("agent runs on %d items in this folder, such as the %s %q, have not finished, and what they write may read data only Admins can query, such as the audit trail; wait for them to end before you share this folder", len(items), first.Kind.Noun(), first.Title)
	}
	return restrictedAnswerError{message}
}

func ShowAnswer(readRestricted, adminsOnly bool) error {
	if readRestricted && !adminsOnly {
		return ErrRestrictedAnswer
	}
	return nil
}

func LinkedRefusal(err error) bool {
	return errors.Is(err, ErrLinkedMove) || errors.Is(err, ErrLinkedObservationMove) || errors.Is(err, ErrLinkedFolder)
}

func AdminsOnly(f dbsqlc.Folder) bool {
	return f.AdminsOnly
}

type ContentKind string

const (
	ContentDashboard   ContentKind = "dashboard"
	ContentObservation ContentKind = "observation"
	ContentSavedSearch ContentKind = "saved_search"
	ContentProposal    ContentKind = "proposal"
	ContentAnswer      ContentKind = "answer"
)

func (k ContentKind) Noun() string {
	switch k {
	case ContentObservation:
		return "observation"
	case ContentSavedSearch:
		return "saved search"
	case ContentProposal:
		return "proposal"
	case ContentAnswer:
		return "answer"
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
	Unlisted   int
	Hidden     int
	message    string
}

func (e *RestrictedContentError) Error() string { return e.message }

func RestrictedSave(kind ContentKind, datasource string) error {
	return &RestrictedContentError{Datasource: datasource, message: fmt.Sprintf(
		"this %s reads the datasource %q, which only Admins can query; save it in a folder only Admins can read", kind.Noun(), datasource)}
}

func RestrictedMove(kind ContentKind, datasource string) error {
	return &RestrictedContentError{Datasource: datasource, message: fmt.Sprintf(
		"this %s reads the datasource %q, which only Admins can query, so it can only move to another folder only Admins can read", kind.Noun(), datasource)}
}

func RestrictedOpening(items []ContentItem, unlisted int, datasource string) error {
	total := len(items) + unlisted
	message := fmt.Sprintf("%d items in this folder read datasources only Admins can query, such as %q; move them to another folder only Admins can read before you share this one", total, datasource)
	if total == 1 {
		item := items[0]
		message = fmt.Sprintf("the %s %q in this folder reads the datasource %q, which only Admins can query; move it to another folder only Admins can read before you share this one", item.Kind.Noun(), item.Title, datasource)
	}
	return &RestrictedContentError{Datasource: datasource, Items: items, Unlisted: unlisted, message: message}
}

func RestrictedAudience(datasource string, items []ContentItem, unlisted, hidden int) error {
	message := fmt.Sprintf("dashboards, observations or saved searches outside folders only Admins can read use the datasource %q; move them to a folder only Admins can read before you make it Admins only", datasource)
	if unlisted > 0 {
		message += fmt.Sprintf("; %d more %s not listed", unlisted, Agree(unlisted, "is", "are"))
	}
	if hidden > 0 {
		message += fmt.Sprintf("; %d of them %s in people's My Dashboards, which only %s can open, so %s must move or remove %s; you cannot see which", hidden, Agree(hidden, "is", "are"),
			Agree(hidden, "its owner", "their owners"), Agree(hidden, "the owner", "the owners"), Agree(hidden, "it", "them"))
	}
	return &RestrictedContentError{Datasource: datasource, Items: items, Unlisted: unlisted, Hidden: hidden, message: message}
}

func Agree(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

type contentItemJSON struct {
	Kind        ContentKind `json:"kind"`
	ID          string      `json:"id"`
	Title       string      `json:"title"`
	FolderID    string      `json:"folder_id"`
	FolderTitle string      `json:"folder_title"`
}

func AbortRestricted(c *gin.Context, err error) bool {
	if errors.Is(err, ErrRestrictedAnswer) {
		httpapi.AbortError(c, http.StatusConflict, CodeRestrictedAnswer, err.Error())
		return true
	}
	if errors.Is(err, ErrProvisioned) {
		httpapi.AbortError(c, http.StatusConflict, CodeProvisioned, err.Error())
		return true
	}
	if LinkedRefusal(err) {
		httpapi.AbortError(c, http.StatusConflict, CodeAdminsOnlyFolder, err.Error())
		return true
	}
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
		body["unlisted"] = restricted.Unlisted
	}
	if restricted.Hidden > 0 {
		body["hidden"] = restricted.Hidden
	}
	c.AbortWithStatusJSON(http.StatusConflict, body)
	return true
}
