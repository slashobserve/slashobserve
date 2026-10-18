set -eu
: "${COMPOSE_PROJECT:?COMPOSE_PROJECT must be set}"
sed "s/__COMPOSE_PROJECT__/${COMPOSE_PROJECT}/g" /etc/prometheus/prometheus.yml.tmpl > /prometheus/prometheus.yml

publish_start() {
  printf '%s\n' "$1" > /prometheus/backfill-end.tmp
  mv /prometheus/backfill-end.tmp /prometheus/backfill-end
}

if [ "${PROMETHEUS_BACKFILL:-1}" = "1" ] && [ ! -f /prometheus/.backfilled ]; then
  tmp=/prometheus/.backfill-tmp
  reuse=0
  if [ -f "$tmp/.ready" ] && [ "$(cat /prometheus/backfill-end 2>/dev/null)" = "$(cat "$tmp/.end")" ]; then
    reuse=1
  else
    rm -f /prometheus/backfill-end
  fi
  lead=20
  # must stay below maxStartLead in demo/shop/internal/loadgen/loadgen.go, or the load generator starts at once and double counts; loadgen_test.go checks it
  max_lead=55
  budget=90
  tries=0
  loop_began=$(date +%s)
  slack() { echo $(($(cat "$tmp/.end") - $(date +%s))); }
  while [ "$reuse" = 0 ] && { [ ! -f "$tmp/.ready" ] || [ "$(slack)" -lt 7 ]; }; do
    if [ "$tries" -ge 4 ] || [ "$(($(date +%s) - loop_began))" -ge "$budget" ]; then
      echo "backfill generation did not finish 7s ahead of its end time within 4 tries or ${budget}s; the junction will show a gap" >&2
      break
    fi
    tries=$((tries + 1))
    rm -rf "$tmp"
    mkdir "$tmp"
    began=$(date +%s)
    end=$((began + lead))
    awk -v now="$end" -f /etc/prometheus/backfill.awk > "$tmp/backfill.om"
    promtool tsdb create-blocks-from openmetrics "$tmp/backfill.om" "$tmp"
    rm -f "$tmp/backfill.om"
    echo "$end" > "$tmp/.end"
    touch "$tmp/.ready"
    lead=$((($(date +%s) - began) * 2 + 10))
    if [ "$lead" -gt "$max_lead" ]; then lead=$max_lead; fi
  done
  end=$(cat "$tmp/.end")
  publish_start "$end"
  remaining=$((end - 5 - $(date +%s)))
  if [ "$remaining" -gt 0 ]; then
    sleep "$remaining"
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
