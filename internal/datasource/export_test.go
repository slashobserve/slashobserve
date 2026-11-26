package datasource

import "time"

func (r *Registry) SetClock(now func() time.Time) { r.now = now }
