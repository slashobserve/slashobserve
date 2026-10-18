package clickhouse

import (
	"context"
	"crypto/tls"
	"strings"
	"sync/atomic"
	"time"

	ch "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"golang.org/x/sync/errgroup"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	defaultDatabase       = "otel"
	defaultMaxOpenConns   = 10
	readOnlyCacheTTL      = 60 * time.Second
	readOnlyTimeout       = 5 * time.Second
	driverDeadlineSlack   = 5 * time.Second
	minQueryTimeout       = time.Second
	readOnlyRefusedCode   = 164
	settingConstraintCode = 452
)

type Options struct {
	Addr         string
	Database     string
	Username     string
	Password     datasource.Secret
	TLS          bool
	Limits       datasource.Limits
	MaxOpenConns int
}

type Datasource struct {
	conn          driver.Conn
	limits        datasource.Limits
	readOnlyUntil atomic.Int64
	maxExecution  atomic.Int64

	tableFunctions atomic.Pointer[tableFunctionSet]
}

var _ datasource.Datasource = (*Datasource)(nil)

func New(opts Options) (*Datasource, error) {
	if opts.Addr == "" {
		return nil, datasource.ConfigErrorf("clickhouse address is required")
	}
	if opts.Username == "" {
		return nil, datasource.ConfigErrorf("clickhouse username is required")
	}
	if opts.Database == "" {
		opts.Database = defaultDatabase
	}
	if opts.MaxOpenConns <= 0 {
		opts.MaxOpenConns = defaultMaxOpenConns
	}
	limits := opts.Limits.WithDefaults(datasource.DefaultLimits)
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	chOpts := &ch.Options{
		Protocol: ch.Native,
		Addr:     []string{opts.Addr},
		Auth: ch.Auth{
			Database: opts.Database,
			Username: opts.Username,
			Password: opts.Password.Reveal(),
		},
		DialTimeout:     5 * time.Second,
		MaxOpenConns:    opts.MaxOpenConns,
		MaxIdleConns:    opts.MaxOpenConns,
		ConnMaxLifetime: 10 * time.Minute,
	}
	if opts.TLS {
		chOpts.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	conn, err := ch.Open(chOpts)
	if err != nil {
		return nil, datasource.ConfigErrorf("invalid clickhouse options")
	}
	return &Datasource{conn: conn, limits: limits}, nil
}

func (d *Datasource) Type() string { return "clickhouse" }

func (d *Datasource) Close() error { return d.conn.Close() }

func (d *Datasource) CheckHealth(ctx context.Context) error {
	budget := d.clampTimeout(readOnlyTimeout)
	timeout := effectiveTimeout(ctx, budget)
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	if err := d.conn.Ping(ctx); err != nil {
		return mapError(ctx, err, timeout)
	}
	return d.verifyReadOnly(ctx, timeout)
}

func (d *Datasource) readOnly(ctx context.Context) error {
	if time.Now().UnixNano() < d.readOnlyUntil.Load() {
		return nil
	}
	budget := d.clampTimeout(readOnlyTimeout)
	timeout := effectiveTimeout(ctx, budget)
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	return d.verifyReadOnly(ctx, timeout)
}

func (d *Datasource) readSettings(ctx context.Context) (level uint64, maxSeconds float64, err error) {
	err = d.conn.QueryRow(ctx, "SELECT toUInt64(getSetting('readonly')), "+
		"toFloat64OrZero(ifNull((SELECT max FROM system.settings WHERE name = 'max_execution_time'), ''))").Scan(&level, &maxSeconds)
	return level, maxSeconds, err
}

func (d *Datasource) verifyReadOnly(ctx context.Context, timeout time.Duration) error {
	callCtx := ctx
	level, maxSeconds, err := d.readSettings(callCtx)
	if exceptionCode(err) == settingConstraintCode {
		// The profile maximum is not known yet; a deadline of 1s or less makes the driver send no max_execution_time.
		short, cancel := context.WithTimeout(ctx, minQueryTimeout)
		defer cancel()
		timeout = effectiveTimeout(ctx, minQueryTimeout)
		callCtx = short
		level, maxSeconds, err = d.readSettings(callCtx)
	}
	if err != nil {
		if exceptionCode(err) != readOnlyRefusedCode {
			return mapError(callCtx, err, timeout)
		}
		level = 1
	}
	switch level {
	case 0:
		d.readOnlyUntil.Store(0)
		return datasource.Errorf(datasource.ErrForbiddenQuery, "datasource user is not read-only")
	case 1:
		d.readOnlyUntil.Store(0)
		return datasource.Errorf(datasource.ErrForbiddenQuery, "read-only user must use readonly=2 so per-query limits can be set")
	}
	d.maxExecution.Store(int64(maxSeconds * float64(time.Second)))
	budget := d.clampTimeout(readOnlyTimeout)
	timeout = effectiveTimeout(ctx, budget)
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	if err := d.verifyNoSourceGrants(ctx, timeout); err != nil {
		d.readOnlyUntil.Store(0)
		return err
	}
	d.loadTableFunctions(ctx)
	d.readOnlyUntil.Store(time.Now().Add(readOnlyCacheTTL).UnixNano())
	return nil
}

// The driver sets max_execution_time to the context's remaining seconds plus a slack, and the
// server refuses a value above the profile maximum, so the deadline has to leave room for it.
func (d *Datasource) clampTimeout(timeout time.Duration) time.Duration {
	limit := time.Duration(d.maxExecution.Load())
	if limit <= 0 {
		return timeout
	}
	return min(timeout, max(limit-driverDeadlineSlack, minQueryTimeout))
}

func effectiveTimeout(ctx context.Context, timeout time.Duration) time.Duration {
	if deadline, ok := ctx.Deadline(); ok {
		return min(timeout, max(time.Until(deadline), 0))
	}
	return timeout
}

const (
	sourceCheckConcurrent = 8
	tempTableGrant        = "CREATE TEMPORARY TABLE"
)

var sourceObjects = []string{
	"ARROW_FLIGHT", "AZURE", "BIGQUERY", "DISK", "FILE", "HDFS", "HIVE", "JDBC", "KAFKA", "MONGO", "MYSQL",
	"NATS", "ODBC", "POSTGRES", "RABBITMQ", "REDIS", "REMOTE", "S3", "SQLITE", "URL", "YTSAURUS",
}

var legacySourceGrants = []string{
	"FILE", "URL", "REMOTE", "MONGO", "REDIS", "MYSQL", "POSTGRES", "SQLITE", "ODBC", "JDBC",
	"HDFS", "S3", "HIVE", "AZURE", "KAFKA", "NATS", "RABBITMQ",
}

func (d *Datasource) checkGrant(ctx context.Context, statement string) (bool, error) {
	var granted uint8
	err := d.conn.QueryRow(ctx, "CHECK GRANT "+statement).Scan(&granted)
	return granted != 0, err
}

// ClickHouse 26.8 models source access as READ/WRITE on source objects, and CHECK GRANT sees
// through roles, wildcards and SOURCES/ALL. Servers that predate it only know "<SOURCE> ON *.*".
func (d *Datasource) grantChecks(ctx context.Context) ([]string, error) {
	_, err := d.checkGrant(ctx, "READ ON URL")
	if err == nil {
		checks := []string{tempTableGrant + " ON *.*"}
		for _, source := range sourceObjects {
			checks = append(checks, "READ ON "+source, "WRITE ON "+source)
		}
		return checks, nil
	}
	if exceptionCode(err) != syntaxErrorCode {
		return nil, err
	}
	if _, err := d.checkGrant(ctx, "URL ON *.*"); err != nil {
		return nil, err
	}
	checks := []string{tempTableGrant + " ON *.*"}
	for _, source := range legacySourceGrants {
		checks = append(checks, source+" ON *.*")
	}
	return checks, nil
}

func (d *Datasource) verifyNoSourceGrants(ctx context.Context, timeout time.Duration) error {
	checks, err := d.grantChecks(ctx)
	if err != nil {
		return mapError(ctx, err, timeout)
	}
	held := make([]bool, len(checks))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(sourceCheckConcurrent)
	for i, check := range checks {
		group.Go(func() error {
			granted, err := d.checkGrant(groupCtx, check)
			held[i] = granted
			return err
		})
	}
	if err := group.Wait(); err != nil {
		return mapError(ctx, err, timeout)
	}
	for i, check := range checks {
		if held[i] {
			return datasource.Errorf(datasource.ErrForbiddenQuery,
				"datasource user holds the %s grant, which lets queries reach external systems or create temporary tables; revoke it (directly, through a role, and any SOURCES, READ, WRITE or ALL grant that implies it)",
				strings.TrimSuffix(check, " ON *.*"))
		}
	}
	return nil
}

func (d *Datasource) loadTableFunctions(ctx context.Context) {
	if d.tableFunctions.Load() != nil {
		return
	}
	rows, err := d.conn.Query(ctx, "SELECT name, lower(name) IN (SELECT lower(name) FROM system.functions) FROM system.table_functions")
	if err != nil {
		return
	}
	defer func() { _ = rows.Close() }()
	var names []string
	alsoScalar := map[string]bool{}
	for rows.Next() {
		var (
			name   string
			scalar bool
		)
		if err := rows.Scan(&name, &scalar); err != nil {
			return
		}
		names = append(names, name)
		if scalar {
			alsoScalar[strings.ToLower(name)] = true
		}
	}
	if rows.Err() != nil || len(names) == 0 {
		return
	}
	d.tableFunctions.Store(newTableFunctionSet(names, alsoScalar))
}
