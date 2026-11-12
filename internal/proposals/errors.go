package proposals

import (
	"errors"
	"fmt"

	"github.com/slashobserve/slashobserve/internal/folders"
)

var (
	ErrNotFound    = errors.New("proposal not found")
	ErrForbidden   = folders.ErrForbidden
	ErrProposeMode = errors.New("this connection is in propose mode, so it cannot merge")

	errLostClose = errors.New("proposal changed before it could be closed")
)

type ChangedError struct {
	Revision int
}

func (e *ChangedError) Error() string {
	return fmt.Sprintf("the proposal has a newer revision %d", e.Revision)
}
