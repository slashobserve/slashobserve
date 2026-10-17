set -eu
: "${COMPOSE_PROJECT:?COMPOSE_PROJECT must be set}"
sed "s/__COMPOSE_PROJECT__/${COMPOSE_PROJECT}/g" /etc/prometheus/prometheus.yml.tmpl > /prometheus/prometheus.yml
if [ "${PROMETHEUS_BACKFILL:-1}" = "1" ] && [ ! -f /prometheus/.backfilled ]; then
  tmp=/prometheus/.backfill-tmp
  if [ ! -f "$tmp/.ready" ]; then
    rm -rf "$tmp"
    mkdir "$tmp"
    awk -v now="$(date +%s)" -f /etc/prometheus/backfill.awk > "$tmp/backfill.om"
    promtool tsdb create-blocks-from openmetrics "$tmp/backfill.om" "$tmp"
    rm -f "$tmp/backfill.om"
    touch "$tmp/.ready"
  fi
  for block in "$tmp"/*; do
    [ -e "$block" ] || continue
    mv "$block" /prometheus/
  done
  touch /prometheus/.backfilled
  rm -rf "$tmp"
fi
exec /bin/prometheus --config.file=/prometheus/prometheus.yml --storage.tsdb.path=/prometheus --storage.tsdb.retention.time=7d "$@"
