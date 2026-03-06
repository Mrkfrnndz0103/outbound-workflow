#!/usr/bin/env sh
set -eu

terminate() {
  kill -TERM "$wf21_pid" "$wf3_pid" 2>/dev/null || true
}

WF3_ENABLE_HEALTH_SERVER="${WF3_ENABLE_HEALTH_SERVER:-false}" /app/workflow-mdt-updates &
wf3_pid=$!

/app/workflow-drive-csv-consolidation &
wf21_pid=$!

trap terminate INT TERM

exit_code=0
while :; do
  if ! kill -0 "$wf21_pid" 2>/dev/null; then
    wait "$wf21_pid" || exit_code=$?
    break
  fi
  if ! kill -0 "$wf3_pid" 2>/dev/null; then
    wait "$wf3_pid" || exit_code=$?
    break
  fi
  sleep 2
done

terminate
wait "$wf21_pid" 2>/dev/null || true
wait "$wf3_pid" 2>/dev/null || true

exit "$exit_code"
