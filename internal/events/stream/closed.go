package stream

import "github.com/gin-gonic/gin"

type closeReason string

const (
	stillOpen            closeReason = ""
	unannounced          closeReason = "unannounced"
	closedStopping       closeReason = "stopping"
	closedLifetime       closeReason = "lifetime"
	closedAccessLost     closeReason = "access_lost"
	closedCheckFailed    closeReason = "check_failed"
	closedReadFailed     closeReason = "read_failed"
	closedSlowSubscriber closeReason = "slow_subscriber"
)

type closedData struct {
	Reason closeReason `json:"reason"`
}

func sayClosed(c *gin.Context, reason closeReason) {
	if reason == unannounced || c.Request.Context().Err() != nil {
		return
	}
	writeEvent(c, "closed", closedData{Reason: reason})
}

func endAfterCheck(valid, failed bool, failuresLeft int) closeReason {
	switch {
	case !valid && failed:
		return unannounced
	case !valid:
		return closedAccessLost
	case failed && failuresLeft <= 0:
		return closedCheckFailed
	}
	return stillOpen
}

const (
	codeTooManyStreams  = "too_many_streams"
	perCaller           = "per_caller"
	serverCapacity      = "server_capacity"
	tooManyEventStreams = "too many open event streams"
)

func abortTooManyStreams(c *gin.Context, status int, reason, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": message, "code": codeTooManyStreams, "reason": reason})
}
