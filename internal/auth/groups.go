package auth

import (
	"encoding/json"
	"slices"
	"strings"
	"unicode"
)

const (
	maxGroupsKept     = 200
	maxGroupNameBytes = 256
)

func validGroupName(name string) bool {
	return len(name) >= 1 && len(name) <= maxGroupNameBytes && !strings.ContainsFunc(name, unicode.IsControl)
}

func groupsClaim(raw json.RawMessage) []string {
	groups := []string{}
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		return groups
	}
	for _, item := range items {
		var name string
		if json.Unmarshal(item, &name) != nil || !validGroupName(name) {
			continue
		}
		groups = append(groups, name)
		if len(groups) == maxGroupsKept {
			break
		}
	}
	return groups
}

func supportsGroupsScope(scopesSupported []string) bool {
	return slices.Contains(scopesSupported, "groups")
}
