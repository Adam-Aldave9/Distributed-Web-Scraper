#!/bin/bash
# Quick API test script using curl

SUPERVISOR_URL="${SUPERVISOR_URL:-http://localhost:8080}"

echo "Creating test job..."
curl -s -X POST "$SUPERVISOR_URL/jobs" \
  -H "Content-Type: application/json" \
  -d '{"name":"Test Job - Books Scraper","url_seed_search":"https://books.toscrape.com/","cron":"0 * * * * *"}' | jq .

echo ""
echo "Check job status: curl $SUPERVISOR_URL/jobs"
