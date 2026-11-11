package provision_test

import (
	"log/slog"
	"sync"
	"testing"

	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/provision"
)

func TestServersProvisioningTogetherCreateEachFileOnce(t *testing.T) {
	r := newRig(t)
	titles := []string{"First", "Second", "Third"}
	for i, title := range titles {
		r.write(t, "root"+string(rune('a'+i))+".json", specJSON(title, "up"))
	}
	other := provision.New(slog.New(slog.DiscardHandler), r.dashboards, folders.NewAccess(r.db.DB), r.db.DB, r.dir)

	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, p := range []*provision.Provisioner{r.provisioner, other} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs[i] = p.Run(t.Context())
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("provisioner %d: %v", i, err)
		}
	}
	for _, title := range titles {
		var count int
		if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM dashboards WHERE title = $1`, title).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Errorf("%d dashboards titled %q, want 1", count, title)
		}
	}
}
