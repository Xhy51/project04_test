package main

import (
	"flag"
	"log"
	"net/http"
	"os"
)

const (
	dbFile   = "crawler.db"
	seedURL  = "https://www.usfca.edu/"
	maxPages = 0
)

func main() {
	var reset = flag.Bool("reset", false, "reset database for a fresh start")

	flag.Parse()
	if *reset {
		log.Println("A fresh start")
		// Remove the old DB file to ensure a fresh crawl.
		os.Remove(dbFile)
	}
	indexer, err := newIndexer(dbFile)
	if err != nil {
		log.Fatalf("Failed to create indexer: %v", err)
	}
	defer indexer.db.Close()

	// Start the Crawler in a background goroutine.
	crawler := NewCrawler(maxPages, indexer)
	go crawler.Start(seedURL)

	// Start the Web UI.
	webUI, err := newWebUI(indexer)
	if err != nil {
		log.Fatalf("Failed to create WebUI: %v", err)
	}

	// Register handlers and start the web server.
	http.HandleFunc("/", webUI.handleSearch)
	http.HandleFunc("/search", webUI.handleSearch)

	log.Println("Starting web server on http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
