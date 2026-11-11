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
}

func AbortRefusal(c *gin.Context, r *Refusal) {
	allowed := r.Allowed
	if allowed == nil {
		allowed = []string{}
	}
	c.AbortWithStatusJSON(http.StatusConflict, refusalJSON{
		Error:          r.Error(),
		Code:           "invalid_transition",
		State:          r.State,
		AllowedActions: allowed,
	})
}
