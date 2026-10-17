#!/bin/sh
set -eu
sed "s/__PASSWORD__/${SLASHOBSERVE_RO_PASSWORD:?}/" /slashobserve/readonly_user.sql \
  | clickhouse client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --multiquery
