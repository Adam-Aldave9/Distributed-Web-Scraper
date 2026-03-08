package Services

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
	DTOs "worker/DTOs"
	Models "worker/Models"

	"github.com/gocolly/colly/v2"
	"gorm.io/gorm"
)

type ScrapeConfig struct {
	ItemSelector     string // css selector for each product card/item container
	TitleSelector    string
	PriceSelector    string
	RatingSelector   string
	URLSelector      string
	NextPageSelector string // css selector for the next page link
	MaxDepth         int    // max pagination pages to follow (0 = unlimited)
}

func DefaultScrapeConfig() ScrapeConfig {
	return ScrapeConfig{
		ItemSelector:     "article.product_pod",
		TitleSelector:    "h3 a",
		PriceSelector:    "p.price_color",
		RatingSelector:   "p.star-rating",
		URLSelector:      "h3 a",
		NextPageSelector: "li.next a",
		MaxDepth:         50,
	}
}

// runScrape blocks until scraping is complete
func RunScrape(db *gorm.DB, payload DTOs.JobPayload, config ScrapeConfig) (int, error) {
	resultsChan := make(chan Models.ScrapedItem, 100)
	var aggregatorWg sync.WaitGroup
	var dbErr error
	totalSaved := 0

	// aggregator goroutine: reads from channel, batches, writes to db
	aggregatorWg.Add(1)
	go func() {
		defer aggregatorWg.Done()
		batch := make([]Models.ScrapedItem, 0, 50)
		batchSize := 50

		flush := func() {
			if len(batch) == 0 {
				return
			}
			result := db.CreateInBatches(batch, len(batch))
			if result.Error != nil {
				log.Printf("[Job %d] Batch write error: %v", payload.JobID, result.Error)
				dbErr = result.Error
			} else {
				totalSaved += len(batch)
				log.Printf("[Job %d] Flushed %d items to DB (total: %d)", payload.JobID, len(batch), totalSaved)
			}
			batch = batch[:0]
		}

		for item := range resultsChan {
			batch = append(batch, item)
			if len(batch) >= batchSize {
				flush()
			}
		}
		flush()
	}()

	c := colly.NewCollector(
		colly.MaxDepth(config.MaxDepth),
		colly.Async(false),
		colly.AllowedDomains("books.toscrape.com"),
	)

	c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: 1,
		Delay:       500 * time.Millisecond,
	})

	website := extractHost(payload.URLSeedSearch)
	pagesVisited := 0

	c.OnHTML(config.ItemSelector, func(e *colly.HTMLElement) {
		title := e.ChildAttr(config.TitleSelector, "title")
		if title == "" {
			title = e.ChildText(config.TitleSelector)
		}

		priceStr := e.ChildText(config.PriceSelector)
		price := parsePrice(priceStr)

		rating := ""
		if config.RatingSelector != "" {
			ratingClass := e.ChildAttr(config.RatingSelector, "class")
			rating = parseRating(ratingClass)
		}

		itemURL := e.ChildAttr(config.URLSelector, "href")
		if itemURL != "" && !strings.HasPrefix(itemURL, "http") {
			itemURL = e.Request.AbsoluteURL(itemURL)
		}

		item := Models.ScrapedItem{
			JobID:     payload.JobID,
			Title:     strings.TrimSpace(title),
			Rating:    strings.TrimSpace(rating),
			Website:   website,
			Price:     price,
			URL:       itemURL,
			ScrapedAt: time.Now(),
		}

		resultsChan <- item
	})

	c.OnHTML(config.NextPageSelector, func(e *colly.HTMLElement) {
		nextLink := e.Attr("href")
		if nextLink == "" {
			return
		}
		if !strings.HasPrefix(nextLink, "http") {
			nextLink = e.Request.AbsoluteURL(nextLink)
		}
		pagesVisited++
		log.Printf("[Job %d] Following pagination to page %d: %s", payload.JobID, pagesVisited+1, nextLink)
		e.Request.Visit(nextLink)
	})

	c.OnRequest(func(r *colly.Request) {
		log.Printf("[Job %d] Visiting: %s", payload.JobID, r.URL.String())
	})

	c.OnError(func(r *colly.Response, err error) {
		log.Printf("[Job %d] Request error on %s: %v", payload.JobID, r.Request.URL, err)
	})

	err := c.Visit(payload.URLSeedSearch)
	if err != nil {
		close(resultsChan)
		aggregatorWg.Wait()
		return totalSaved, fmt.Errorf("colly visit error: %w", err)
	}

	c.Wait()

	close(resultsChan)
	aggregatorWg.Wait()

	if dbErr != nil {
		return totalSaved, fmt.Errorf("database write errors occurred: %w", dbErr)
	}

	log.Printf("[Job %d] Scraping complete. %d pages visited, %d items saved.", payload.JobID, pagesVisited+1, totalSaved)
	return totalSaved, nil
}

// parsePrice extracts a float64 from a price string like $12.34
func parsePrice(s string) float64 {
	cleaned := strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || r == '.' {
			return r
		}
		return -1
	}, s)
	if cleaned == "" {
		return 0
	}
	price, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return 0
	}
	return price
}

// extractHost returns the hostname from a URL string
func extractHost(rawURL string) string {
	s := rawURL
	if idx := strings.Index(s, "://"); idx != -1 {
		s = s[idx+3:]
	}
	if idx := strings.Index(s, "/"); idx != -1 {
		s = s[:idx]
	}
	return s
}

// parseRating extracts the rating word from a class string like "star-rating Three"
func parseRating(class string) string {
	parts := strings.Fields(class)
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}
