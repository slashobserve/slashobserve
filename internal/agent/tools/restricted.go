package tools

import (
	"errors"

	"github.com/slashobserve/slashobserve/internal/folders"
)

const codeRestrictedContent = folders.CodeRestrictedContent

func restrictedRefusal(err error) (Result, bool) {
	var restricted *folders.RestrictedContentError
	if !errors.As(err, &restricted) {
		return Result{}, false
	}
	return failure("Save rejected: "+codeRestrictedContent, errorBody{Code: codeRestrictedContent, Message: restricted.Error()}), true
}
