CREATE TABLE IF NOT EXISTS otel.otel_logs
(
    Timestamp DateTime64(9),
    ServiceName LowCardinality(String),
    SeverityText LowCardinality(String),
    Body String,
    ResourceAttributes Map(LowCardinality(String), String),
    LogAttributes Map(LowCardinality(String), String)
)
ENGINE = MergeTree
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, Timestamp);

CREATE TABLE IF NOT EXISTS otel.otel_metrics_gauge
(
    ResourceAttributes Map(LowCardinality(String), String),
    ServiceName LowCardinality(String),
    MetricName String,
    MetricDescription String,
    MetricUnit String,
    Attributes Map(LowCardinality(String), String),
    TimeUnix DateTime64(9),
    Value Float64
)
ENGINE = MergeTree
PARTITION BY toDate(TimeUnix)
ORDER BY (ServiceName, MetricName, TimeUnix);

CREATE TABLE IF NOT EXISTS otel.otel_metrics_sum
(
    ResourceAttributes Map(LowCardinality(String), String),
    ServiceName LowCardinality(String),
    MetricName String,
    MetricDescription String,
    MetricUnit String,
    Attributes Map(LowCardinality(String), String),
    TimeUnix DateTime64(9),
    Value Float64,
    IsMonotonic Bool
)
ENGINE = MergeTree
PARTITION BY toDate(TimeUnix)
ORDER BY (ServiceName, MetricName, TimeUnix);

CREATE TABLE IF NOT EXISTS otel.otel_traces
(
    Timestamp DateTime64(9),
    TraceId String,
    SpanId String,
    SpanName LowCardinality(String),
    ServiceName LowCardinality(String),
    Duration UInt64,
    StatusCode LowCardinality(String),
    SpanAttributes Map(LowCardinality(String), String),
    ResourceAttributes Map(LowCardinality(String), String)
)
ENGINE = MergeTree
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, SpanName, Timestamp);
