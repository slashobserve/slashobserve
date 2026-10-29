package imports

import "github.com/slashobserve/slashobserve/internal/auth"

var RespondImported = respondImported

var ListParamsOf = listParamsOf

func (s *Service) HoldPreviewSlot(by auth.Principal) (release func(), held bool) {
	return s.previewSlots.acquire(principalKey(by))
}

func (s *Service) HoldImportSlot(by auth.Principal) (release func(), held bool) {
	return s.importSlots.acquire(principalKey(by))
}
