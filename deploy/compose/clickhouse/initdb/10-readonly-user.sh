#!/bin/sh
set -eu
case "${SLASHOBSERVE_RO_PASSWORD:?}" in
  *[!A-Za-z0-9._-]*)
    echo "SLASHOBSERVE_RO_PASSWORD may contain only letters, digits, '.', '_' and '-'" >&2
    exit 1
    ;;
esac
sed "s/__PASSWORD__/${SLASHOBSERVE_RO_PASSWORD}/" /slashobserve/readonly_user.sql \
  | clickhouse client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --multiquery
