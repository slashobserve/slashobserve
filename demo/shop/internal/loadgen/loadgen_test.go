package loadgen

import (
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"
)

func TestStartLeadCoversEntrypointCap(t *testing.T) {
	script, err := os.ReadFile("../../../../deploy/compose/prometheus/entrypoint.sh")
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?m)^\s*max_lead=(\d+)\s*$`).FindSubmatch(script)
	if match == nil {
		t.Fatal("entrypoint.sh has no max_lead assignment")
	}
	seconds, err := strconv.Atoi(string(match[1]))
	if err != nil {
		t.Fatal(err)
	}
	if lead := time.Duration(seconds) * time.Second; lead >= maxStartLead {
		t.Fatalf("entrypoint max_lead %s must stay below the loadgen's maxStartLead %s", lead, maxStartLead)
	}
	if !regexp.MustCompile(`(?m)^\s*if \[ "\$lead" -gt "\$max_lead" \]; then lead=\$max_lead; fi\s*$`).Match(script) {
		t.Fatal("entrypoint.sh no longer clamps lead to max_lead")
	}
}
