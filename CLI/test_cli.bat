@echo off
REM Quick API test script using curl

if "%SUPERVISOR_URL%"=="" set SUPERVISOR_URL=http://localhost:8080

echo Creating test job...
curl -s -X POST "%SUPERVISOR_URL%/jobs" -H "Content-Type: application/json" -d "{\"name\":\"Test Job - Books Scraper\",\"url_seed_search\":\"https://books.toscrape.com/\",\"cron\":\"0 * * * * *\"}"

echo.
echo Check job status: curl %SUPERVISOR_URL%/jobs

pause
