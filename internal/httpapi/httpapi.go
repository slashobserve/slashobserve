package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const maxCursorMicros = 253402300799999999

func AbortError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": message, "code": code})
}

type InvalidSpecBody struct {
	Error  string       `json:"error"`
	Code   string       `json:"code"`
	Issues []spec.Issue `json:"issues"`
}

func AbortInvalidSpec(c *gin.Context, message string, issues []spec.Issue) {
	c.AbortWithStatusJSON(http.StatusUnprocessableEntity, InvalidSpecBody{Error: message, Code: "invalid_spec", Issues: issues})
}

func DecodeBody(c *gin.Context, dst any, limit int64) bool {
	dec := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, limit))
	dec.DisallowUnknownFields()
	err := dec.Decode(dst)
	if err == nil {
		_, err = dec.Token()
		if errors.Is(err, io.EOF) {
			return true
		}
		if err == nil {
			AbortError(c, http.StatusBadRequest, "invalid_request", "request body must be a single JSON object")
			return false
		}
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		AbortError(c, http.StatusRequestEntityTooLarge, "too_large", "request body is too large")
		return false
	}
	AbortError(c, http.StatusBadRequest, "invalid_request", "request body must be valid JSON with known fields")
	return false
}

func Int32(n int) int32 {
	if n < math.MinInt32 || n > math.MaxInt32 {
		panic("httpapi: integer outside the int32 range")
	}
	return int32(n)
}

func Caller(c *gin.Context) auth.Principal {
	p, _ := auth.FromContext(c.Request.Context())
	return p
}

func UUIDParam(c *gin.Context, name string) (pgtype.UUID, bool) {
	id, err := db.ParseUUID(c.Param(name))
	if err != nil {
		AbortError(c, http.StatusBadRequest, "invalid_request", name+" must be a UUID")
		return pgtype.UUID{}, false
	}
	return id, true
}

func LimitParam(c *gin.Context, fallback, ceiling int) (int, bool) {
	raw, present := c.GetQuery("limit")
	if !present {
		return fallback, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > ceiling || strconv.Itoa(n) != raw {
		AbortError(c, http.StatusBadRequest, "invalid_request", "limit must be an integer from 1 to "+strconv.Itoa(ceiling))
		return 0, false
	}
	return n, true
}

func FormatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func OptionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

type TimeCursor struct {
	Time time.Time
	ID   pgtype.UUID
}

func EncodeTimeCursor(t time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(t.UnixMicro(), 10) + ":" + id))
}

func DecodeTimeCursor(raw string) (*TimeCursor, bool) {
	if raw == "" {
		return nil, true
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, false
	}
	micros, idText, ok := strings.Cut(string(decoded), ":")
	if !ok {
		return nil, false
	}
	n, err := strconv.ParseInt(micros, 10, 64)
	if err != nil || n < 0 || n > maxCursorMicros || strconv.FormatInt(n, 10) != micros {
		return nil, false
	}
	id, err := db.ParseUUID(idText)
	if err != nil {
		return nil, false
	}
	return &TimeCursor{Time: time.UnixMicro(n).UTC(), ID: id}, true
}
