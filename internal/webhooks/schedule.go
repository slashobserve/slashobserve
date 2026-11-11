package webhooks

import "time"

const (
	failingAfter = 50
	expiry       = 24 * time.Hour

	firstRetry    = 5 * time.Second
	longestRetry  = 15 * time.Minute
	shortestAsked = 5 * time.Second
	longestAsked  = time.Hour
)

func nextAttempt(failures, status int, retryAfter time.Duration) time.Duration {
	if (status == 429 || status == 503) && retryAfter > 0 {
		return min(max(retryAfter, shortestAsked), longestAsked)
	}
	delay := firstRetry
	for range max(failures-1, 0) {
		delay *= 2
		if delay >= longestRetry {
			return longestRetry
		}
	}
	return delay
}
