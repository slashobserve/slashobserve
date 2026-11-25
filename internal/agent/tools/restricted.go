package tools

import (
	"errors"

	"github.com/slashobserve/slashobserve/internal/folders"
)

const (
	codeProvisioned       = folders.CodeProvisioned
	codeRestrictedContent = folders.CodeRestrictedContent
	provisionedHint       = " (" + nameDuplicateDashboard + " makes an editable copy)"
	restrictedContentHint = "; use a folder whose admins_only is true (see " + nameListFolders + "), or set a folder to Only Admins with `observe folders permissions admins-only`"
)

func restrictedRefusal(err error) (Result, bool) {
	if errors.Is(err, folders.ErrRestrictedAnswer) {
		return toolError(folders.CodeRestrictedAnswer, err.Error()), true
	}
	if folders.LinkedRefusal(err) {
		return toolError(folders.CodeAdminsOnlyFolder, err.Error()), true
	}
	if errors.Is(err, folders.ErrProvisioned) {
		return failure("Save rejected: "+codeProvisioned, errorBody{Code: codeProvisioned, Message: err.Error() + provisionedHint}), true
	}
	var restricted *folders.RestrictedContentError
	if !errors.As(err, &restricted) {
		return Result{}, false
	}
	return failure("Save rejected: "+codeRestrictedContent, errorBody{Code: codeRestrictedContent, Message: restricted.Error() + restrictedContentHint}), true
}
