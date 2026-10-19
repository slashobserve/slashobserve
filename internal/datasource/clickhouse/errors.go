package clickhouse

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	ch "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const maxExceptionMessage = 512

var (
	addressToken = regexp.MustCompile(`[0-9A-Za-z:.\[\]%_-]+`)
	hostPort     = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*(\.[A-Za-z0-9_-]+)*:\d{2,5}$`)
)

// Exceptions a query author can act on and that never carry infrastructure details by themselves.
var queryErrorCodes = map[int32]bool{
	6: true, 10: true, 16: true, 27: true, 34: true, 35: true, 36: true, 38: true, 41: true, 42: true,
	43: true, 44: true, 46: true, 47: true, 48: true, 53: true, 59: true, 60: true, 62: true, 63: true,
	69: true, 70: true, 81: true, 117: true, 153: true, 179: true, 184: true, 190: true, 215: true,
	352: true, 386: true, 395: true, 407: true, 427: true,
}

func mapError(ctx context.Context, err error, timeout time.Duration) error {
	if err == nil {
		return nil
	}
	var ue *datasource.UpstreamError
	if errors.As(err, &ue) {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return datasource.ContextError(ctx, err, timeout)
	}
	var ex *ch.Exception
	if errors.As(err, &ex) {
		return mapException(ex)
	}
	if isTransportError(err) {
		if cerr := datasource.ContextError(ctx, err, timeout); cerr != nil {
			return cerr
		}
		return datasource.Errorf(datasource.ErrUnavailable, "connection failed")
	}
	return datasource.Errorf(datasource.ErrInvalidQuery, "query result could not be read")
}

func mapException(ex *ch.Exception) error {
	switch ex.Code {
	case 159:
		return datasource.Errorf(datasource.ErrTimeout, "%s", safeMessage(ex, "query timed out"))
	case 158, 241, 307, 396:
		return datasource.Errorf(datasource.ErrLimitExceeded, "%s", safeMessage(ex, "query limit exceeded"))
	case 452:
		return datasource.Errorf(datasource.ErrLimitExceeded, "a configured limit violates a ClickHouse profile constraint: %s", safeMessage(ex, "setting constraint violated"))
	case 164, 497:
		return datasource.Errorf(datasource.ErrForbiddenQuery, "not allowed for the read-only user")
	case 516, 192, 193, 194:
		return datasource.Errorf(datasource.ErrAuthFailed, "authentication failed")
	case 195:
		return datasource.Errorf(datasource.ErrHostNotAllowed, "connection not allowed for the datasource user")
	case 209, 210, 279, 519:
		return datasource.Errorf(datasource.ErrUnavailable, "connection to a ClickHouse shard failed")
	}
	if queryErrorCodes[ex.Code] {
		return datasource.Errorf(datasource.ErrInvalidQuery, "%s", safeMessage(ex, fmt.Sprintf("query failed (ClickHouse error %d)", ex.Code)))
	}
	return datasource.Errorf(datasource.ErrInvalidQuery, "query failed (ClickHouse error %d)", ex.Code)
}

func safeMessage(ex *ch.Exception, fallback string) string {
	if leaksAddress(ex.Message) {
		return fallback
	}
	return truncate(ex.Message)
}

func leaksAddress(message string) bool {
	for _, token := range addressToken.FindAllString(message, -1) {
		token = strings.Trim(token, ".:,")
		if token == "" {
			continue
		}
		if _, err := netip.ParseAddr(token); err == nil {
			return true
		}
		if _, err := netip.ParseAddrPort(token); err == nil || hostPort.MatchString(token) {
			return true
		}
	}
	return false
}

func isTransportError(err error) bool {
	var (
		netErr     net.Error
		recordErr  tls.RecordHeaderError
		alertErr   tls.AlertError
		verifyErr  *tls.CertificateVerificationError
		authority  x509.UnknownAuthorityError
		hostname   x509.HostnameError
		invalid    x509.CertificateInvalidError
		systemRoot x509.SystemRootsError
	)
	switch {
	case errors.As(err, &netErr), errors.As(err, &recordErr), errors.As(err, &alertErr), errors.As(err, &verifyErr),
		errors.As(err, &authority), errors.As(err, &hostname), errors.As(err, &invalid), errors.As(err, &systemRoot):
		return true
	}
	for _, target := range []error{
		io.EOF, io.ErrUnexpectedEOF, net.ErrClosed, syscall.ECONNREFUSED, syscall.ECONNRESET, syscall.EPIPE,
		ch.ErrAcquireConnTimeout, ch.ErrAcquireConnNoAddress, ch.ErrConnectionClosed, ch.ErrUnsupportedServerRevision,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

func truncate(s string) string {
	if len(s) <= maxExceptionMessage {
		return s
	}
	s = s[:maxExceptionMessage]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

func exceptionCode(err error) int32 {
	var ex *ch.Exception
	if errors.As(err, &ex) {
		return ex.Code
	}
	return 0
}

func messageOf(err error) string {
	var ue *datasource.UpstreamError
	if errors.As(err, &ue) {
		return ue.Message
	}
	return err.Error()
}
