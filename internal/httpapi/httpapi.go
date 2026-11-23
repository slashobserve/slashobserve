package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/usertext"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const maxCursorMicros = 253402300799999999

func AbortError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": message, "code": code})
}

func AbortText(c *gin.Context, status int, code string, err error) {
	body := gin.H{"error": err.Error(), "code": code}
	if reason, ok := usertext.ReasonOf(err); ok {
		body["reason"] = reason
	}
	c.AbortWithStatusJSON(status, body)
}

type InvalidSpecBody struct {
	Error  string       `json:"error"`
	Code   string       `json:"code"`
	Issues []spec.Issue `json:"issues"`
	Reason string       `json:"reason,omitempty"`
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
	AbortError(c, http.StatusBadRequest, "invalid_request", decodeProblem(err))
	return false
}

func decodeProblem(err error) string {
	var wrongType *json.UnmarshalTypeError
	var syntax *json.SyntaxError
	switch {
	case errors.Is(err, io.EOF):
		return "request body is empty; send a JSON object"
	case errors.As(err, &wrongType) && wrongType.Field != "":
		return fmt.Sprintf("%s must be %s, not %s", wrongType.Field, jsonKindWord(wrongType.Type), wrongType.Value)
	case errors.As(err, &syntax), errors.As(err, &wrongType), errors.Is(err, io.ErrUnexpectedEOF):
		return "request body is not valid JSON; send one JSON object"
	}
	if name, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
		return fmt.Sprintf("unknown field %s; remove it or check the field names in the API reference", name)
	}
	return "request body must be valid JSON with known fields"
}

func jsonKindWord(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "a whole number"
	case reflect.Float32, reflect.Float64:
		return "a number"
	case reflect.String:
		return "a string"
	case reflect.Bool:
		return "true or false"
	case reflect.Slice, reflect.Array:
		return "a list"
	default:
		return "an object"
	}
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

func NonEmptyQuery(c *gin.Context, name string) (string, bool) {
	value, present := c.GetQuery(name)
	if present && value == "" {
		AbortError(c, http.StatusBadRequest, "invalid_request", name+" must not be empty")
		return "", false
	}
	return value, true
}

func OffsetParam(c *gin.Context, ceiling int) (int, bool) {
	raw, present := c.GetQuery("offset")
	if !present {
		return 0, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 || n > ceiling || strconv.Itoa(n) != raw {
		AbortError(c, http.StatusBadRequest, "invalid_request", "offset must be an integer from 0 to "+strconv.Itoa(ceiling))
		return 0, false
	}
	return n, true
}

func FormatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func FormatOptionalTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := FormatTime(*t)
	return &s
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
