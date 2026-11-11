package activity

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/actor"
)

const (
	TypeDashboardCreated        = "dashboard.created"
	TypeDashboardVersionCreated = "dashboard.version_created"
	TypeDashboardRestored       = "dashboard.restored"
	TypeDashboardMoved          = "dashboard.moved"
	TypeDashboardDeleted        = "dashboard.deleted"

	TypeAnnotationCreated = "annotation.created"
	TypeAnnotationDeleted = "annotation.deleted"

	TypeFindingCreated = "finding.created"
	TypeFindingPinned  = "finding.pinned"

	TypeThreadOpened    = "thread.opened"
	TypeThreadReplied   = "thread.replied"
	TypeThreadEdited    = "thread.edited"
	TypeThreadResolved  = "thread.resolved"
	TypeThreadReopened  = "thread.reopened"
	TypeThreadMentioned = "thread.mentioned"

	TypeRunQueued    = "run.queued"
	TypeRunStarted   = "run.started"
	TypeRunSucceeded = "run.succeeded"
	TypeRunFailed    = "run.failed"
	TypeRunCanceled  = "run.canceled"
)

var knownTypes = []string{
	TypeDashboardCreated,
	TypeDashboardVersionCreated,
	TypeDashboardRestored,
	TypeDashboardMoved,
	TypeDashboardDeleted,
	TypeAnnotationCreated,
	TypeAnnotationDeleted,
	TypeFindingCreated,
	TypeFindingPinned,
	TypeThreadOpened,
	TypeThreadReplied,
	TypeThreadEdited,
	TypeThreadResolved,
	TypeThreadReopened,
	TypeThreadMentioned,
	TypeRunQueued,
	TypeRunStarted,
	TypeRunSucceeded,
	TypeRunFailed,
	TypeRunCanceled,
}

func Types() []string { return slices.Clone(knownTypes) }

func Known(t string) bool { return slices.Contains(knownTypes, t) }

func targetKind(t string) string {
	kind, _, _ := strings.Cut(t, ".")
	return kind
}

type Event struct {
	Seq         int64
	Type        string
	TargetKind  string
	TargetID    string
	DashboardID string
	FolderID    string
	Actor       actor.Actor
	Data        json.RawMessage
	CreatedAt   time.Time
	Position    Position
}

type Position struct {
	xid uint64
	seq int64
}

func At(xid uint64, seq int64) Position { return Position{xid: xid, seq: seq} }

func (p Position) Xid() uint64 { return p.xid }

func (p Position) Seq() int64 { return p.seq }

func later(a, b Position) Position {
	if b.xid > a.xid || (b.xid == a.xid && b.seq > a.seq) {
		return b
	}
	return a
}
