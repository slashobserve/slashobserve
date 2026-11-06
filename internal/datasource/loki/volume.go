package loki

import (
	"strconv"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const volumeNeedsLogQuery = `the volume needs a log query that starts with a stream selector, such as {service_name="checkout"}`

func VolumeQuery(query string, step time.Duration) (string, error) {
	if !isLogQuery(query) {
		return "", datasource.Errorf(datasource.ErrInvalidQuery, "%s", volumeNeedsLogQuery)
	}
	return "sum by (" + strings.Join(levelLabels, ", ") + ") (count_over_time(\n" + query + "\n[" + strconv.FormatInt(step.Milliseconds(), 10) + "ms]))", nil
}

func VolumeLevel(labels map[string]string) string {
	level, _ := streamLevel(labels)
	return level
}
