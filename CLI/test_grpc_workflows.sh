#!/bin/bash
# Test script: verify gRPC workflows (register, heartbeat, shutdown) via REST API
# The REST API exposes the state produced by gRPC calls, so we can validate
# the full gRPC lifecycle without a gRPC client tool.

SUPERVISOR_URL="${SUPERVISOR_URL:-http://localhost:8080}"
PASS_COUNT=0
FAIL_COUNT=0

# ── Helpers ──────────────────────────────────────────────────────────
header() {
  echo ""
  echo "╔══════════════════════════════════════════════════════════╗"
  printf "║  %-55s ║\n" "$1"
  echo "╚══════════════════════════════════════════════════════════╝"
}

pass() { PASS_COUNT=$((PASS_COUNT + 1)); echo "  PASS: $1"; }
fail() { FAIL_COUNT=$((FAIL_COUNT + 1)); echo "  FAIL: $1"; }

pyjson() {
  python3 -c "
import sys, json
data = json.load(sys.stdin)
$1
"
}

# ── Preflight: supervisor reachable ─────────────────────────────────
header "Preflight: checking supervisor"

WORKERS_RAW=$(curl -sf "$SUPERVISOR_URL/workers")
if [ $? -ne 0 ]; then
  echo "  Cannot reach supervisor at $SUPERVISOR_URL"
  echo "  Make sure docker compose is running."
  exit 1
fi
echo "  Supervisor reachable at $SUPERVISOR_URL"

# ═══════════════════════════════════════════════════════════════════
# TEST 1: gRPC RegisterWorker
# Workers call RegisterWorker on startup. If GET /workers returns
# active workers, the gRPC registration succeeded.
# ═══════════════════════════════════════════════════════════════════
header "Test 1: gRPC RegisterWorker (worker registration)"

WORKER_COUNT=$(echo "$WORKERS_RAW" | pyjson "print(len(data))")

if [ "$WORKER_COUNT" -lt 1 ]; then
  fail "No workers registered. Start at least one worker and re-run."
  echo ""
  echo "Results: $PASS_COUNT passed, $FAIL_COUNT failed"
  exit 1
fi

pass "$WORKER_COUNT worker(s) registered via gRPC"

# Verify each worker has expected fields from RegisterWorker RPC
echo "$WORKERS_RAW" | pyjson "
for w in data:
    print(f\"  - id={w['id']}  name={w['name']}  status={w['status']}  host={w['host_name']}\")
"

ACTIVE_COUNT=$(echo "$WORKERS_RAW" | pyjson "print(sum(1 for w in data if w['status'] == 'active'))")
if [ "$ACTIVE_COUNT" -ge 1 ]; then
  pass "$ACTIVE_COUNT worker(s) have status=active (set by RegisterWorker RPC)"
else
  fail "No workers with status=active"
fi

# Verify workers have a hostname (populated by RegisterWorker RPC)
MISSING_HOST=$(echo "$WORKERS_RAW" | pyjson "print(sum(1 for w in data if not w.get('host_name')))")
if [ "$MISSING_HOST" -eq 0 ]; then
  pass "All workers have host_name set (populated by RegisterWorker RPC)"
else
  fail "$MISSING_HOST worker(s) missing host_name"
fi

# ═══════════════════════════════════════════════════════════════════
# TEST 2: gRPC Heartbeat
# Workers send Heartbeat every 15s. We verify last_heartbeat is
# recent and updates between two checks.
# ═══════════════════════════════════════════════════════════════════
header "Test 2: gRPC Heartbeat (periodic health updates)"

# Pick the first active worker
TARGET_WORKER=$(echo "$WORKERS_RAW" | pyjson "
w = next((w for w in data if w['status'] == 'active'), None)
if w:
    print(f\"{w['id']}|{w['name']}\")
else:
    print('')
")

if [ -z "$TARGET_WORKER" ]; then
  fail "No active worker to test heartbeat against"
else
  WORKER_ID=$(echo "$TARGET_WORKER" | cut -d'|' -f1)
  WORKER_NAME=$(echo "$TARGET_WORKER" | cut -d'|' -f2)
  echo "  Testing heartbeat on worker $WORKER_NAME (id=$WORKER_ID)"

  # Get initial heartbeat timestamp
  WORKER_DETAIL=$(curl -sf "$SUPERVISOR_URL/workers/$WORKER_ID")
  HB1=$(echo "$WORKER_DETAIL" | pyjson "print(data.get('last_heartbeat', ''))")
  echo "  Initial last_heartbeat: $HB1"

  # Check heartbeat is recent (within 60 seconds)
  IS_RECENT=$(echo "$WORKER_DETAIL" | pyjson "
from datetime import datetime, timezone, timedelta
hb = data.get('last_heartbeat', '')
try:
    ts = datetime.fromisoformat(hb.replace('Z', '+00:00'))
    diff = (datetime.now(timezone.utc) - ts).total_seconds()
    print('yes' if diff < 60 else 'no')
except:
    print('no')
")

  if [ "$IS_RECENT" = "yes" ]; then
    pass "last_heartbeat is within 60s (Heartbeat RPC is active)"
  else
    fail "last_heartbeat is stale (>60s old). Heartbeat RPC may not be working."
  fi

  # Wait for a heartbeat cycle and check timestamp updates
  echo "  Waiting 16s for next heartbeat cycle..."
  sleep 16

  WORKER_DETAIL2=$(curl -sf "$SUPERVISOR_URL/workers/$WORKER_ID")
  HB2=$(echo "$WORKER_DETAIL2" | pyjson "print(data.get('last_heartbeat', ''))")
  echo "  Updated last_heartbeat: $HB2"

  if [ "$HB1" != "$HB2" ]; then
    pass "last_heartbeat updated between checks (Heartbeat RPC confirmed working)"
  else
    fail "last_heartbeat did not update after 16s. Heartbeat RPC may be stalled."
  fi

  # Verify heartbeat carries capacity info
  HAS_CAPACITY=$(echo "$WORKER_DETAIL2" | pyjson "print('yes' if data.get('capacity', 0) > 0 else 'no')")
  if [ "$HAS_CAPACITY" = "yes" ]; then
    CAPACITY=$(echo "$WORKER_DETAIL2" | pyjson "print(data['capacity'])")
    pass "Worker reports capacity=$CAPACITY (sent via Heartbeat RPC)"
  else
    fail "Worker capacity is 0 or missing"
  fi
fi

# ═══════════════════════════════════════════════════════════════════
# TEST 3: gRPC Shutdown
# POST /workers/:id/shutdown triggers the Shutdown RPC on the worker.
# We verify the worker transitions to offline/shutdown status.
# ═══════════════════════════════════════════════════════════════════
header "Test 3: gRPC Shutdown (remote worker shutdown)"

# Only test shutdown if there are 2+ active workers (don't kill the only one)
ACTIVE_WORKERS=$(curl -sf "$SUPERVISOR_URL/workers" | pyjson "
workers = [w for w in data if w['status'] == 'active']
print(len(workers))
")

if [ "$ACTIVE_WORKERS" -lt 2 ]; then
  echo "  SKIP: Only $ACTIVE_WORKERS active worker(s). Need 2+ to safely test shutdown."
  echo "  (Skipping to avoid killing the only running worker)"
else
  # Pick the last active worker to shutdown
  SHUTDOWN_TARGET=$(curl -sf "$SUPERVISOR_URL/workers" | pyjson "
workers = [w for w in data if w['status'] == 'active']
w = workers[-1]
print(f\"{w['id']}|{w['name']}\")
")
  SHUTDOWN_ID=$(echo "$SHUTDOWN_TARGET" | cut -d'|' -f1)
  SHUTDOWN_NAME=$(echo "$SHUTDOWN_TARGET" | cut -d'|' -f2)

  echo "  Sending shutdown to worker $SHUTDOWN_NAME (id=$SHUTDOWN_ID)..."
  SHUTDOWN_RESP=$(curl -sf -X POST "$SUPERVISOR_URL/workers/$SHUTDOWN_ID/shutdown")
  echo "  Response: $SHUTDOWN_RESP"

  SHUTDOWN_MSG=$(echo "$SHUTDOWN_RESP" | pyjson "print(data.get('message', ''))")
  if echo "$SHUTDOWN_MSG" | grep -qi "shutdown"; then
    pass "Shutdown RPC accepted (response: $SHUTDOWN_MSG)"
  else
    fail "Unexpected shutdown response: $SHUTDOWN_MSG"
  fi

  # Wait for shutdown to take effect
  echo "  Waiting 5s for shutdown to propagate..."
  sleep 5

  # Check worker status changed
  WORKER_AFTER=$(curl -sf "$SUPERVISOR_URL/workers/$SHUTDOWN_ID")
  STATUS_AFTER=$(echo "$WORKER_AFTER" | pyjson "print(data.get('status', ''))")

  if [ "$STATUS_AFTER" != "active" ]; then
    pass "Worker status changed to '$STATUS_AFTER' after Shutdown RPC"
  else
    fail "Worker still shows status='active' after shutdown"
  fi
fi

# ═══════════════════════════════════════════════════════════════════
# Results
# ═══════════════════════════════════════════════════════════════════
header "Results"

TOTAL=$((PASS_COUNT + FAIL_COUNT))
echo "  $PASS_COUNT/$TOTAL passed"

if [ "$FAIL_COUNT" -gt 0 ]; then
  echo "  $FAIL_COUNT test(s) FAILED"
  exit 1
else
  echo "  All gRPC workflow tests passed!"
fi

echo ""
