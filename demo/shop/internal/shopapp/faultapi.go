package shopapp

import (
	"net/http"
	"strconv"

	"github.com/slashobserve/slashobserve/demo/shop/internal/fault"
)

type faultState struct {
	Pod        string `json:"pod"`
	Service    string `json:"service"`
	ErrorsPct  int    `json:"errors_pct"`
	LatencyPct int    `json:"latency_pct"`
}

func (a *app) faultState() faultState {
	return faultState{
		Pod:        a.cfg.Pod,
		Service:    a.cfg.Service,
		ErrorsPct:  a.faults.Pct(fault.Errors),
		LatencyPct: a.faults.Pct(fault.Latency),
	}
}

func (a *app) getFault(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, a.faultState())
}

func (a *app) setFault(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	kind, err := fault.ParseKind(query.Get("type"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	pct := fault.DefaultPct
	if raw := query.Get("pct"); raw != "" {
		pct, err = strconv.Atoi(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "pct must be an integer between 0 and 100")
			return
		}
	}
	if err := a.faults.Set(kind, pct); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	a.log.WarnContext(r.Context(), "fault changed", "fault.type", string(kind), "fault.pct", pct)
	writeJSON(w, http.StatusOK, a.faultState())
}

func (a *app) resetFault(w http.ResponseWriter, r *http.Request) {
	a.faults.Reset()
	for _, kind := range []fault.Kind{fault.Errors, fault.Latency} {
		a.log.WarnContext(r.Context(), "fault changed", "fault.type", string(kind), "fault.pct", 0)
	}
	writeJSON(w, http.StatusOK, a.faultState())
}
