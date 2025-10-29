package main

import (
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/net/html"
)

const (
	Delay              = 100 * time.Millisecond
	FetcherConcurrency = 32
)

type fetchIO struct {
	urls      <-chan string
	responses chan<- Response
}

func fetcher(wg *sync.WaitGroup, host string, io fetchIO) {
	defer wg.Done()
	ticker := time.NewTicker(Delay)
	visited := make(map[string]struct{})
	urlQueue := make(chan string, FetcherConcurrency)
	var internalGroup sync.WaitGroup

	internalGroup.Add(FetcherConcurrency)
	for i := 0; i < FetcherConcurrency; i++ {
		go func() {
			for url_ := range urlQueue {
				// Enforce ethical delay
				<-ticker.C

				// Send HTTP request
				r, err := http.Get(url_)
				if err != nil {
					// Skip failed URL
					log.Printf("Error fetching %s: %v\n", url_, err)
					return
				}

				// Parse HTML doc
				doc, err := html.Parse(r.Body)
				r.Body.Close()
				if err != nil {
					// Skip failed page
					log.Printf("Error parsing %s: %v\n", url_, err)
					return
				}

				log.Printf("Fetched %s: %v\n", url_, r.StatusCode)
				if r.StatusCode == http.StatusOK {
					io.responses <- Response{url_, doc}
				}
			}
			internalGroup.Done()
		}()
	}

	for url_ := range io.urls {
		u, err := url.Parse(url_)
		if err != nil || u.Host != host {
			// Drop URLs that are invalid or with wrong hosts
			continue
		}
		if _, ok := visited[url_]; ok {
			continue
		}
		urlQueue <- url_
		visited[url_] = struct{}{}
	}
	close(urlQueue)
	internalGroup.Wait()
}
