package tools

import (
	"errors"

	"github.com/slashobserve/slashobserve/internal/folders"
)

const (
	codeRestrictedContent = folders.CodeRestrictedContent
	restrictedContentHint = "; use a folder whose admins_only is true (see " + nameListFolders + "), or ask an Admin to set a folder to Only Admins with `observe folders permissions admins-only`"
)

func restrictedRefusal(err error) (Result, bool) {
	if errors.Is(err, folders.ErrRestrictedAnswer) {
		return toolError(folders.CodeRestrictedAnswer, err.Error()), true
	}
	if folders.LinkedRefusal(err) {
		return toolError(folders.CodeAdminsOnlyFolder, err.Error()), true
	}
	var restricted *folders.RestrictedContentError
	if !errors.As(err, &restricted) {
		return Result{}, false
	}
	return failure("Save rejected: "+codeRestrictedContent, errorBody{Code: codeRestrictedContent, Message: restricted.Error() + restrictedContentHint}), true
}
