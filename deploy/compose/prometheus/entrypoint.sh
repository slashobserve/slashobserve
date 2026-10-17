set -eu
: "${COMPOSE_PROJECT:?COMPOSE_PROJECT must be set}"
sed "s/__COMPOSE_PROJECT__/${COMPOSE_PROJECT}/g" /etc/prometheus/prometheus.yml.tmpl > /prometheus/prometheus.yml
if [ "${PROMETHEUS_BACKFILL:-1}" = "1" ] && [ ! -f /prometheus/.backfilled ]; then
  awk -v now="$(date +%s)" -f /etc/prometheus/backfill.awk > /prometheus/backfill.om
  promtool tsdb create-blocks-from openmetrics /prometheus/backfill.om /prometheus
  rm -f /prometheus/backfill.om
  touch /prometheus/.backfilled
fi
exec /bin/prometheus --config.file=/prometheus/prometheus.yml --storage.tsdb.path=/prometheus --storage.tsdb.retention.time=7d "$@"
