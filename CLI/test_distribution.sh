#!/bin/bash
# Test script: verify jobs get distributed across multiple workers
# Uses python3 for JSON parsing (no jq dependency)

SUPERVISOR_URL="${SUPERVISOR_URL:-http://localhost:8080}"
POLL_INTERVAL=3
MAX_POLLS=20

# ── Helpers ──────────────────────────────────────────────────────────
header() {
  echo ""
  echo "╔══════════════════════════════════════════════════════════╗"
  printf "║  %-55s ║\n" "$1"
  echo "╚══════════════════════════════════════════════════════════╝"
}

fail() { echo "FAIL: $1"; exit 1; }

# JSON helper: extract fields using python3
pyjson() {
  python3 -c "
import sys, json
data = json.load(sys.stdin)
$1
"
}

# ── Step 1: Check registered workers ────────────────────────────────
header "Step 1: Checking registered workers"

WORKERS=$(curl -sf "$SUPERVISOR_URL/workers")
[ $? -ne 0 ] && fail "Cannot reach supervisor at $SUPERVISOR_URL/workers"

WORKER_COUNT=$(echo "$WORKERS" | pyjson "print(len(data))")
echo "Registered workers: $WORKER_COUNT"
echo "$WORKERS" | pyjson "
for w in data:
    print(f\"  - id={w['id']}  name={w['name']}  status={w['status']}  active_jobs={w['active_jobs']}  capacity={w['capacity']}\")
"

if [ "$WORKER_COUNT" -lt 2 ]; then
  echo ""
  echo "WARNING: Only $WORKER_COUNT worker(s) registered."
  echo "  Distribution requires at least 2 workers."
  echo "  Start more workers and re-run this script."
  exit 1
fi

# ── Step 2: Submit jobs (one more than worker count) ────────────────
JOB_COUNT=$((WORKER_COUNT + 1))
header "Step 2: Submitting $JOB_COUNT jobs"

CATEGORIES=(
  "Full Catalog|https://books.toscrape.com/"
  "Science Books|https://books.toscrape.com/catalogue/category/books/science_22/index.html"
  "Travel Books|https://books.toscrape.com/catalogue/category/books/travel_2/index.html"
  "Mystery Books|https://books.toscrape.com/catalogue/category/books/mystery_3/index.html"
  "History Books|https://books.toscrape.com/catalogue/category/books/history_32/index.html"
  "Fantasy Books|https://books.toscrape.com/catalogue/category/books/fantasy_19/index.html"
)

SUBMITTED_IDS=()

for i in $(seq 0 $((JOB_COUNT - 1))); do
  idx=$((i % ${#CATEGORIES[@]}))
  IFS='|' read -r NAME URL <<< "${CATEGORIES[$idx]}"

  RESP=$(curl -sf -X POST "$SUPERVISOR_URL/jobs" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"Dist Test - $NAME\",\"url_seed_search\":\"$URL\",\"cron\":\"0 * * * * *\"}")

  JOB_ID=$(echo "$RESP" | pyjson "print(data.get('id', data.get('Id', '')))")
  if [ -n "$JOB_ID" ]; then
    SUBMITTED_IDS+=("$JOB_ID")
    echo "  Submitted job $JOB_ID: $NAME"
  else
    echo "  Failed to submit: $NAME"
    echo "  Response: $RESP"
  fi
done

echo ""
echo "Submitted ${#SUBMITTED_IDS[@]} jobs: ${SUBMITTED_IDS[*]}"

# ── Step 3: Poll until jobs are picked up or completed ──────────────
header "Step 3: Waiting for jobs to be picked up by workers"

POLL=0
ALL_ASSIGNED=false

while [ $POLL -lt $MAX_POLLS ] && [ "$ALL_ASSIGNED" = false ]; do
  POLL=$((POLL + 1))
  sleep $POLL_INTERVAL

  JOBS=$(curl -sf "$SUPERVISOR_URL/jobs")
  IDS_STR="${SUBMITTED_IDS[*]}"

  POLL_RESULT=$(echo "$JOBS" | python3 -c "
import sys, json
data = json.load(sys.stdin)
ids = [int(x) for x in '$IDS_STR'.split()]
all_assigned = True
for jid in ids:
    job = next((j for j in data if j['id'] == jid), None)
    if job:
        status = job['status']
        wid = job.get('worker_id', '')
        print(f'  Job {jid:<4}  status={status:<12}  worker_id={wid if wid else \"-\"}')
        if status not in ('completed', 'running') and not wid:
            all_assigned = False
    else:
        print(f'  Job {jid:<4}  NOT FOUND')
        all_assigned = False
print(f'__ASSIGNED__={all_assigned}')
")

  echo "── Poll $POLL/$MAX_POLLS ──"
  echo "$POLL_RESULT" | grep -v '^__ASSIGNED__'

  if echo "$POLL_RESULT" | grep -q '__ASSIGNED__=True'; then
    ALL_ASSIGNED=true
    echo ""
    echo "All jobs have been assigned or completed."
  fi
done

if [ "$ALL_ASSIGNED" = false ]; then
  echo ""
  echo "WARNING: Not all jobs were assigned within the polling window."
fi

# ── Step 4: Distribution report ─────────────────────────────────────
header "Step 4: Distribution report"

JOBS=$(curl -sf "$SUPERVISOR_URL/jobs")
IDS_STR="${SUBMITTED_IDS[*]}"

REPORT=$(echo "$JOBS" | python3 -c "
import sys, json
data = json.load(sys.stdin)
ids = [int(x) for x in '$IDS_STR'.split()]
worker_jobs = {}
for jid in ids:
    job = next((j for j in data if j['id'] == jid), None)
    if job:
        wid = job.get('worker_id', '')
        status = job['status']
        name = job['name']
        print(f'  Job {jid:<4}  {name:<30}  status={status:<12}  worker={wid if wid else \"-\"}')
        if wid:
            worker_jobs.setdefault(wid, []).append(f'{jid}({status})')

print()
print(f'Workers that handled jobs: {len(worker_jobs)}')
for wid, jobs in worker_jobs.items():
    print(f'  Worker {wid} -> jobs: {\", \".join(jobs)}')
print(f'__UNIQUE_COUNT__={len(worker_jobs)}')
wids = list(worker_jobs.keys())
if wids:
    print(f'__FIRST_WORKER__={wids[0]}')
")

echo "$REPORT" | grep -v '^__'

UNIQUE_COUNT=$(echo "$REPORT" | grep '__UNIQUE_COUNT__' | cut -d= -f2)
FIRST_WORKER=$(echo "$REPORT" | grep '__FIRST_WORKER__' | cut -d= -f2)

# ── Step 5: Verdict ─────────────────────────────────────────────────
header "Step 5: Verdict"

if [ "$UNIQUE_COUNT" -ge 2 ]; then
  echo "  PASS: Jobs were distributed across $UNIQUE_COUNT workers."
elif [ "$UNIQUE_COUNT" -eq 1 ]; then
  echo "  FAIL: All jobs went to a single worker ($FIRST_WORKER)."
  echo "        Check that multiple workers are running and polling Redis."
else
  echo "  FAIL: No jobs were assigned to any worker."
  echo "        Check that workers are running and connected."
fi

echo ""
