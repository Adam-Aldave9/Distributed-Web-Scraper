#!/bin/bash
# Test script: heartbeat resilience — reconnect, re-registration, dead worker detection + job requeue
# Requires: docker compose running with at least 2 workers scaled
# Usage: bash test_heartbeat_resilience.sh
# NOTE: this script stops/starts worker containers. It will restore them at the end.

SUPERVISOR_URL="${SUPERVISOR_URL:-http://localhost:8080}"
COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.yml}"
WORKER_SERVICE="${WORKER_SERVICE:-worker}"
PASS_COUNT=0
FAIL_COUNT=0
SKIP_COUNT=0

# ── Helpers ──────────────────────────────────────────────────────────
header() {
  echo ""
  echo "╔══════════════════════════════════════════════════════════╗"
  printf "║  %-55s ║\n" "$1"
  echo "╚══════════════════════════════════════════════════════════╝"
}

pass() { PASS_COUNT=$((PASS_COUNT + 1)); echo "  PASS: $1"; }
fail() { FAIL_COUNT=$((FAIL_COUNT + 1)); echo "  FAIL: $1"; }
skip() { SKIP_COUNT=$((SKIP_COUNT + 1)); echo "  SKIP: $1"; }

pyjson() {
  python3 -c "
import sys, json
data = json.load(sys.stdin)
$1
"
}

# polls GET /workers/:id until status matches or timeout
# usage: wait_for_status <worker_id> <expected_status> <timeout_seconds>
wait_for_status() {
  local wid=$1 expected=$2 timeout=$3
  local elapsed=0
  while [ $elapsed -lt $timeout ]; do
    local status
    status=$(curl -sf "$SUPERVISOR_URL/workers/$wid" | pyjson "print(data.get('status', ''))")
    if [ "$status" = "$expected" ]; then
      return 0
    fi
    sleep 3
    elapsed=$((elapsed + 3))
  done
  return 1
}

# ── Preflight ────────────────────────────────────────────────────────
header "Preflight"

WORKERS_RAW=$(curl -sf "$SUPERVISOR_URL/workers")
if [ $? -ne 0 ]; then
  echo "  Cannot reach supervisor at $SUPERVISOR_URL"
  exit 1
fi
echo "  Supervisor reachable"

ACTIVE_COUNT=$(echo "$WORKERS_RAW" | pyjson "print(sum(1 for w in data if w['status'] == 'active'))")
echo "  Active workers: $ACTIVE_COUNT"

if [ "$ACTIVE_COUNT" -lt 2 ]; then
  echo "  Need at least 2 active workers. Scale up and re-run."
  echo "  e.g.: docker compose up --scale worker=2 -d"
  exit 1
fi

# pick the last active worker as our target — we'll be stopping it
TARGET=$(echo "$WORKERS_RAW" | pyjson "
workers = [w for w in data if w['status'] == 'active']
w = workers[-1]
print(f\"{w['id']}|{w['name']}\")
")
TARGET_ID=$(echo "$TARGET" | cut -d'|' -f1)
TARGET_NAME=$(echo "$TARGET" | cut -d'|' -f2)
echo "  Target worker: $TARGET_NAME (id=$TARGET_ID)"

# worker name format is "worker-<container_short_id>" — extract the ID and match to docker container
CONTAINER_SHORT_ID=$(echo "$TARGET_NAME" | sed 's/^worker-//')
CONTAINER=$(docker ps --filter "label=com.docker.compose.service=${WORKER_SERVICE}" --format '{{.Names}} {{.ID}}' \
  | grep "$CONTAINER_SHORT_ID" | awk '{print $1}')
if [ -z "$CONTAINER" ]; then
  echo "  Could not find docker container for worker $TARGET_NAME (short id: $CONTAINER_SHORT_ID)"
  echo "  Set WORKER_SERVICE env var if your compose service name differs"
  exit 1
fi
echo "  Docker container: $CONTAINER"

# snapshot initial heartbeat
INITIAL_HB=$(curl -sf "$SUPERVISOR_URL/workers/$TARGET_ID" | pyjson "print(data.get('last_heartbeat', ''))")
echo "  Initial heartbeat: $INITIAL_HB"

# ═══════════════════════════════════════════════════════════════════
# TEST 1: Heartbeat stops when worker container is paused
# Pausing the container simulates a network partition — the worker
# process is alive but can't reach the supervisor.
# ═══════════════════════════════════════════════════════════════════
header "Test 1: Heartbeat stops on network partition (pause)"

echo "  Pausing container $CONTAINER..."
docker pause "$CONTAINER" > /dev/null 2>&1
if [ $? -ne 0 ]; then
  skip "docker pause failed — may need elevated permissions"
else
  echo "  Waiting 80s for heartbeat timeout (45s) + health check cycle (30s)..."
  sleep 80

  STATUS_AFTER_PAUSE=$(curl -sf "$SUPERVISOR_URL/workers/$TARGET_ID" | pyjson "print(data.get('status', ''))")
  if [ "$STATUS_AFTER_PAUSE" = "offline" ]; then
    pass "Worker marked offline after heartbeat timeout"
  else
    fail "Worker status is '$STATUS_AFTER_PAUSE', expected 'offline'"
  fi

  # check heartbeat is stale
  HB_STALE=$(curl -sf "$SUPERVISOR_URL/workers/$TARGET_ID" | pyjson "
from datetime import datetime, timezone
hb = data.get('last_heartbeat', '')
try:
    ts = datetime.fromisoformat(hb.replace('Z', '+00:00'))
    diff = (datetime.now(timezone.utc) - ts).total_seconds()
    print('yes' if diff > 45 else 'no')
except:
    print('error')
")
  if [ "$HB_STALE" = "yes" ]; then
    pass "Heartbeat timestamp is stale (>45s old)"
  else
    fail "Heartbeat timestamp is unexpectedly fresh"
  fi

  echo "  Unpausing container $CONTAINER..."
  docker unpause "$CONTAINER" > /dev/null 2>&1
fi

# ═══════════════════════════════════════════════════════════════════
# TEST 2: Worker auto-reconnects and re-registers after unpause
# After unpausing, the worker's heartbeat should start failing
# (connection lost), trigger reconnect, then re-register since
# supervisor won't acknowledge heartbeat for an offline worker.
# ═══════════════════════════════════════════════════════════════════
header "Test 2: Auto-reconnect and re-registration after recovery"

echo "  Waiting up to 90s for worker to come back online..."
if wait_for_status "$TARGET_ID" "active" 90; then
  pass "Worker re-registered and returned to active status"

  # confirm heartbeat is fresh again
  HB_FRESH=$(curl -sf "$SUPERVISOR_URL/workers/$TARGET_ID" | pyjson "
from datetime import datetime, timezone
hb = data.get('last_heartbeat', '')
try:
    ts = datetime.fromisoformat(hb.replace('Z', '+00:00'))
    diff = (datetime.now(timezone.utc) - ts).total_seconds()
    print('yes' if diff < 30 else 'no')
except:
    print('error')
")
  if [ "$HB_FRESH" = "yes" ]; then
    pass "Heartbeat is fresh after reconnect (<30s old)"
  else
    fail "Heartbeat is still stale after reconnect"
  fi
else
  fail "Worker did not return to active within 90s"
fi

# ═══════════════════════════════════════════════════════════════════
# TEST 3: Dead worker job requeue
# Submit a job, wait for target worker to pick it up, then kill
# the worker. The supervisor health check should detect the dead
# worker and requeue the job for another worker.
# ═══════════════════════════════════════════════════════════════════
header "Test 3: Dead worker job requeue"

# make sure target is active before proceeding
CURRENT_STATUS=$(curl -sf "$SUPERVISOR_URL/workers/$TARGET_ID" | pyjson "print(data.get('status', ''))")
if [ "$CURRENT_STATUS" != "active" ]; then
  skip "Target worker is not active ($CURRENT_STATUS), cannot test job requeue"
else
  # submit a job
  JOB_RESP=$(curl -sf -X POST "$SUPERVISOR_URL/jobs" \
    -H "Content-Type: application/json" \
    -d '{"name":"Heartbeat Requeue Test","url_seed_search":"https://books.toscrape.com/catalogue/category/books/history_32/index.html","cron":"0 * * * * *"}')

  JOB_ID=$(echo "$JOB_RESP" | pyjson "print(data.get('id', data.get('Id', '')))")
  echo "  Submitted job $JOB_ID"

  # wait for job to be picked up
  echo "  Waiting for job to be assigned..."
  JOB_ASSIGNED=false
  for i in $(seq 1 15); do
    sleep 2
    JOB_STATUS=$(curl -sf "$SUPERVISOR_URL/jobs/$JOB_ID" | pyjson "
status = data.get('status', '')
wid = data.get('worker_id', '')
print(f'{status}|{wid}')
")
    STATUS=$(echo "$JOB_STATUS" | cut -d'|' -f1)
    WORKER_ID=$(echo "$JOB_STATUS" | cut -d'|' -f2)

    if [ "$STATUS" = "completed" ]; then
      skip "Job completed before we could kill the worker — too fast. Try a larger seed URL."
      JOB_ASSIGNED=done
      break
    fi

    if [ "$STATUS" = "running" ] && [ -n "$WORKER_ID" ]; then
      echo "  Job $JOB_ID running on worker $WORKER_ID"
      JOB_ASSIGNED=true
      break
    fi
  done

  if [ "$JOB_ASSIGNED" = "true" ]; then
    # kill the worker that picked it up
    KILL_CONTAINER="$CONTAINER"
    echo "  Stopping container $KILL_CONTAINER to simulate crash..."
    docker stop "$KILL_CONTAINER" > /dev/null 2>&1

    echo "  Waiting 50s for health check to detect dead worker..."
    sleep 50

    # check that the job was requeued (status back to pending or picked up by another worker)
    JOB_AFTER=$(curl -sf "$SUPERVISOR_URL/jobs/$JOB_ID" | pyjson "
status = data.get('status', '')
wid = data.get('worker_id', '')
print(f'{status}|{wid}')
")
    AFTER_STATUS=$(echo "$JOB_AFTER" | cut -d'|' -f1)
    AFTER_WORKER=$(echo "$JOB_AFTER" | cut -d'|' -f2)

    if [ "$AFTER_STATUS" = "pending" ]; then
      pass "Job $JOB_ID requeued to pending after worker death"
    elif [ "$AFTER_STATUS" = "running" ] && [ "$AFTER_WORKER" != "$WORKER_ID" ]; then
      pass "Job $JOB_ID picked up by different worker ($AFTER_WORKER) after requeue"
    elif [ "$AFTER_STATUS" = "completed" ]; then
      pass "Job $JOB_ID completed (another worker picked it up after requeue)"
    else
      fail "Job $JOB_ID status is '$AFTER_STATUS' with worker '$AFTER_WORKER' — expected requeue"
    fi

    # check dead worker is offline
    DEAD_STATUS=$(curl -sf "$SUPERVISOR_URL/workers/$TARGET_ID" | pyjson "print(data.get('status', ''))")
    if [ "$DEAD_STATUS" = "offline" ]; then
      pass "Dead worker correctly marked offline"
    else
      fail "Dead worker status is '$DEAD_STATUS', expected 'offline'"
    fi

    # restart the killed container
    echo "  Restarting container $KILL_CONTAINER..."
    docker start "$KILL_CONTAINER" > /dev/null 2>&1

    echo "  Waiting up to 60s for worker to re-register..."
    if wait_for_status "$TARGET_ID" "active" 60; then
      pass "Worker re-registered after restart"
    else
      echo "  Worker may register as a new entry — checking all workers..."
      NEW_ACTIVE=$(curl -sf "$SUPERVISOR_URL/workers" | pyjson "print(sum(1 for w in data if w['status'] == 'active'))")
      if [ "$NEW_ACTIVE" -ge "$ACTIVE_COUNT" ]; then
        pass "Worker count restored ($NEW_ACTIVE active)"
      else
        fail "Worker did not re-register. Active: $NEW_ACTIVE, expected: $ACTIVE_COUNT"
      fi
    fi

  elif [ "$JOB_ASSIGNED" != "done" ]; then
    fail "Job was never assigned to a worker within 30s"
  fi
fi

# ═══════════════════════════════════════════════════════════════════
# TEST 4: Heartbeat resumes correctly after brief supervisor restart
# Restart just the supervisor and verify workers reconnect.
# ═══════════════════════════════════════════════════════════════════
header "Test 4: Worker heartbeat survives supervisor restart"

SUPERVISOR_CONTAINER=$(docker ps --filter "label=com.docker.compose.service=supervisor" --format '{{.Names}}' | head -1)
if [ -z "$SUPERVISOR_CONTAINER" ]; then
  skip "Could not find supervisor container"
else
  echo "  Restarting supervisor container $SUPERVISOR_CONTAINER..."
  docker restart "$SUPERVISOR_CONTAINER" > /dev/null 2>&1

  # wait for supervisor API to come back
  echo "  Waiting for supervisor API..."
  for i in $(seq 1 20); do
    sleep 3
    if curl -sf "$SUPERVISOR_URL/workers" > /dev/null 2>&1; then
      echo "  Supervisor API is back"
      break
    fi
  done

  # workers should reconnect and re-register
  echo "  Waiting up to 90s for workers to re-register..."
  RECOVERED=false
  for i in $(seq 1 30); do
    sleep 3
    CURRENT_ACTIVE=$(curl -sf "$SUPERVISOR_URL/workers" 2>/dev/null | pyjson "print(sum(1 for w in data if w['status'] == 'active'))" 2>/dev/null)
    if [ "$CURRENT_ACTIVE" -ge "$ACTIVE_COUNT" ]; then
      RECOVERED=true
      break
    fi
  done

  if [ "$RECOVERED" = true ]; then
    pass "Workers re-registered after supervisor restart ($CURRENT_ACTIVE active)"

    # verify heartbeats are fresh
    sleep 20
    ALL_FRESH=$(curl -sf "$SUPERVISOR_URL/workers" | pyjson "
from datetime import datetime, timezone
count = 0
for w in data:
    if w['status'] != 'active':
        continue
    hb = w.get('last_heartbeat', '')
    try:
        ts = datetime.fromisoformat(hb.replace('Z', '+00:00'))
        if (datetime.now(timezone.utc) - ts).total_seconds() < 30:
            count += 1
    except:
        pass
print(count)
")
    if [ "$ALL_FRESH" -ge "$ACTIVE_COUNT" ]; then
      pass "All $ALL_FRESH workers have fresh heartbeats after supervisor restart"
    else
      fail "Only $ALL_FRESH/$ACTIVE_COUNT workers have fresh heartbeats"
    fi
  else
    fail "Workers did not re-register within 90s after supervisor restart"
  fi
fi

# ═══════════════════════════════════════════════════════════════════
# Results
# ═══════════════════════════════════════════════════════════════════
header "Results"

TOTAL=$((PASS_COUNT + FAIL_COUNT + SKIP_COUNT))
echo "  $PASS_COUNT passed, $FAIL_COUNT failed, $SKIP_COUNT skipped (of $TOTAL)"

if [ "$FAIL_COUNT" -gt 0 ]; then
  echo "  Some tests FAILED"
  exit 1
else
  echo "  All heartbeat resilience tests passed!"
fi

echo ""
