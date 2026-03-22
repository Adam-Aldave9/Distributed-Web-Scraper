# Distributed Web Scraper

A distributed web scraping system built using **Supervisor/Worker** architecture. The Supervisor accepts scraping jobs via REST API, queues them in Redis, and dispatches them to one or more Worker nodes. Workers scrape the target sites using Colly and write results directly to PostgreSQL. Workers register themselves with the Supervisor via gRPC and send periodic heartbeats so the Supervisor can detect and recover from failures automatically.

![Architecture Diagram](DWS-Diagram.png)

## Job Distribution

- Supervisor pushes job payloads onto a Redis list (`LPush`).
- Each worker caps concurrent scrapes with a buffered channel used as a semaphore.
- Workers block on `BRPop` and whoever pops first claims the job, allowing Redis to handle routing with no central dispatcher needed.
- A `sync.WaitGroup` ensures in-flight jobs finish before shutdown.

## Technologies

| Layer | Tech |
|---|---|
| Language | Go |
| REST API | Gin |
| Inter-service communication | gRPC / Protocol Buffers |
| Job queue | Redis |
| Scraping | Colly |
| Database | PostgreSQL (x3) + GORM |
| Containerisation | Docker / Docker Compose |

## Scaling Workers

Workers are stateless and can be scaled up at any time without restarting the system. Each new worker automatically registers itself with the Supervisor via gRPC and begins consuming jobs from the Redis queue immediately.

**Start with multiple workers:**
```bash
docker compose up --build --scale worker=3
```

**Scale up while the system is running:**
```bash
docker compose up --scale worker=5 -d
```
