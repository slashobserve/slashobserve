package lifecycle

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type refusalJSON struct {
	Error          string   `json:"error"`
	Code           string   `json:"code"`
	State          string   `json:"state"`
	AllowedActions []string `json:"allowed_actions"`
	Reason         string   `json:"reason,omitempty"`
}

const CodeInvalidTransition = "invalid_transition"

func AbortRefusal(c *gin.Context, r *Refusal) {
	allowed := r.Allowed
	if allowed == nil {
		allowed = []string{}
	}
	code := r.Code
	if code == "" {
		code = CodeInvalidTransition
	}
	c.AbortWithStatusJSON(http.StatusConflict, refusalJSON{
		Error:          r.Error(),
		Code:           code,
		State:          r.State,
		AllowedActions: allowed,
		Reason:         r.Reason,
	})
}
