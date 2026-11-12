package proposals

import (
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func mergeTakingTheirs(base, ours, theirs *spec.Dashboard) (spec.Merge, error) {
	first, err := spec.Merge3(base, ours, theirs)
	if err != nil || len(first.Conflicts) == 0 {
		return first, err
	}
	var choices []spec.Choice
	for _, c := range first.Conflicts {
		if c.Reason == spec.ReasonBothChanged || c.Reason == spec.ReasonChangedAndRemoved {
			choices = append(choices, spec.Choice{Kind: c.Kind, ID: c.ID, Take: spec.SideTheirs})
		}
	}
	if len(choices) == 0 {
		return first, nil
	}
	return spec.Merge3(base, ours, theirs, choices...)
}

func fold(headBase, headSpec, saveBase, saveSpec *spec.Dashboard, headBaseVersion, saveBaseVersion int) (*spec.Dashboard, int, error) {
	head := headSpec
	if saveBaseVersion > headBaseVersion {
		rebased, err := mergeTakingTheirs(headBase, saveBase, headSpec)
		if err != nil {
			return nil, 0, err
		}
		if len(rebased.Conflicts) > 0 {
			return saveSpec, saveBaseVersion, nil
		}
		head = rebased.Dashboard
	}
	folded, err := mergeTakingTheirs(saveBase, head, saveSpec)
	if err != nil {
		return nil, 0, err
	}
	if len(folded.Conflicts) > 0 {
		return saveSpec, saveBaseVersion, nil
	}
	return folded.Dashboard, max(headBaseVersion, saveBaseVersion), nil
}
