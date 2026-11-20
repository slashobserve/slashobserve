package revisions

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
)

type SharedTitle string

const (
	TitleRenamed SharedTitle = "renamed"
	TitleKept    SharedTitle = "kept"
)

type TitleSharer func(ctx context.Context, by auth.Principal, chatID pgtype.UUID, title string) (SharedTitle, error)
