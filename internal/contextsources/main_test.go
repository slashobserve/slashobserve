package contextsources

import (
	"os"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.MainOnDemand(m))
}
