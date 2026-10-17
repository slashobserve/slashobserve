package clickhouse

import (
	"context"
	"crypto/tls"
	"sync/atomic"
	"time"

	ch "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	defaultDatabase     = "otel"
	defaultMaxOpenConns = 10
	readOnlyCacheTTL    = 60 * time.Second
	readOnlyTimeout     = 5 * time.Second
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
}

var _ datasource.Datasource = (*Datasource)(nil)

func New(opts Options) (*Datasource, error) {
	if opts.Addr == "" {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "clickhouse address is required")
	}
	if opts.Username == "" {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "clickhouse username is required")
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
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "invalid clickhouse options")
	}
	return &Datasource{conn: conn, limits: limits}, nil
}

func (d *Datasource) Type() string { return "clickhouse" }

func (d *Datasource) Close() error { return d.conn.Close() }

func (d *Datasource) CheckHealth(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, readOnlyTimeout)
	defer cancel()
	if err := d.conn.Ping(ctx); err != nil {
		return mapError(err, readOnlyTimeout)
	}
	return d.verifyReadOnly(ctx)
}

func (d *Datasource) readOnly(ctx context.Context) error {
	if time.Now().UnixNano() < d.readOnlyUntil.Load() {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, readOnlyTimeout)
	defer cancel()
	return d.verifyReadOnly(ctx)
}

func (d *Datasource) verifyReadOnly(ctx context.Context) error {
	var level uint64
	if err := d.conn.QueryRow(ctx, "SELECT toUInt64(getSetting('readonly'))").Scan(&level); err != nil {
		return mapError(err, readOnlyTimeout)
	}
	if level == 0 {
		d.readOnlyUntil.Store(0)
		return datasource.Errorf(datasource.ErrForbiddenQuery, "datasource user is not read-only")
	}
	d.readOnlyUntil.Store(time.Now().Add(readOnlyCacheTTL).UnixNano())
	return nil
}
