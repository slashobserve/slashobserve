package server

import "github.com/slashobserve/slashobserve/api/apitest"

func init() {
	loadAPIDocument = apitest.Document
}
