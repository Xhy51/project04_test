package main

import (
	"log"
	"net/url"
	"sync"
)

const (
	PageBufferSize = 10240
)

// Crawler manages the crawling process.
type Crawler struct {
	maxPages int
	indexer  *Indexer
}

// NewCrawler creates a new Crawler instance.
func NewCrawler(maxPages int, indexer *Indexer) *Crawler {
	return &Crawler{
		maxPages: maxPages,
		indexer:  indexer,
	}
}

// Start begins the crawling process from a seed URL.
func (c *Crawler) Start(seedURL string) {
	seed, err := url.Parse(seedURL)
	if err != nil {
		return
	}

	pages := make(chan Page, PageBufferSize)
	quit := make(chan struct{})
	d := newDispatcher()
	var wg sync.WaitGroup

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go extracter(&wg, d.responses, pages)
	}

	d.dispatch(seedURL)

	go func() {
		d.wait()
		wg.Wait()
		quit <- struct{}{}
		close(pages)
	}()

	dispatched := 1
	for {
		select {
		case page := <-pages:
			visited, err := c.indexer.queryUrlVisited(page.Url)
			if err != nil {
				log.Printf("Failed to dedup page %s: %v\n", page.Url, err)
				continue
			}
			if visited {
				log.Printf("Skip to index page %s\n", page.Url)
			} else if err = c.indexer.addPage(page); err != nil {
				log.Printf("Failed to index page %s: %v\n", page.Url, err)
			}

			for _, u := range page.ContainedUrls {
				url_, err := url.Parse(u)
				if err != nil || url_.Host != seed.Host {
					continue
				}
				if c.maxPages == 0 || dispatched < c.maxPages {
					d.dispatch(u)
					dispatched++
					if dispatched == c.maxPages {
						d.close()
					}
				}
			}
		case <-quit:
			log.Printf("Crawler finished")
			return
		}
	}
}
