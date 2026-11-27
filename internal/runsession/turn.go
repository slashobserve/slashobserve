package runsession

import "github.com/slashobserve/slashobserve/internal/revisions"

type Turn struct {
	ID    string `json:"id"`
	Calls int    `json:"calls"`
	Saves int    `json:"saves"`

	Usage *revisions.Usage `json:"usage"`
}
