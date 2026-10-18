set -eu
: "${COMPOSE_PROJECT:?COMPOSE_PROJECT must be set}"
sed "s/__COMPOSE_PROJECT__/${COMPOSE_PROJECT}/g" /etc/prometheus/prometheus.yml.tmpl > /prometheus/prometheus.yml

publish_start() {
  printf '%s\n' "$1" > /prometheus/backfill-end.tmp
  mv /prometheus/backfill-end.tmp /prometheus/backfill-end
}

if [ "${PROMETHEUS_BACKFILL:-1}" = "1" ] && [ ! -f /prometheus/.backfilled ]; then
  tmp=/prometheus/.backfill-tmp
  if [ ! -f "$tmp/.ready" ]; then
    rm -rf "$tmp"
    mkdir "$tmp"
    end=$(($(date +%s) + 20))
    awk -v now="$end" -f /etc/prometheus/backfill.awk > "$tmp/backfill.om"
    promtool tsdb create-blocks-from openmetrics "$tmp/backfill.om" "$tmp"
    rm -f "$tmp/backfill.om"
    echo "$end" > "$tmp/.end"
    touch "$tmp/.ready"
  fi
  end=$(cat "$tmp/.end")
  publish_start "$end"
  remaining=$((end - 5 - $(date +%s)))
  if [ "$remaining" -gt 0 ]; then
    sleep "$remaining"
  elif [ "$remaining" -lt -5 ]; then
    echo "backfill generation overran its 20s lead by $((-remaining - 5))s; the junction will show a gap" >&2
  fi
  for block in "$tmp"/*; do
    [ -e "$block" ] || continue
    mv "$block" /prometheus/
  done
  touch /prometheus/.backfilled
  rm -rf "$tmp"
else
  publish_start "$(date +%s)"
fi
exec /bin/prometheus --config.file=/prometheus/prometheus.yml --storage.tsdb.path=/prometheus --storage.tsdb.retention.time=7d "$@"
