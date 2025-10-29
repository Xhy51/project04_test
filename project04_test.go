package main

import (
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
)

func TestTfIdf(t *testing.T) {
	// Setup a mock HTTP server to serve an expanded set of test pages.
	// This server simulates a small website with five pages to test various TF-IDF cases.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		var pageContent string
		switch r.URL.Path {
		case "/":
			// Page 1: "apple banana"
			pageContent = `<html><head><title>Page 1</title></head><body><p>apple banana</p><a href="/page2">2</a></body></html>`
		case "/page2":
			// Page 2: "apple cherry"
			pageContent = `<html><head><title>Page 2</title></head><body><p>apple cherry</p><a href="/page3">3</a></body></html>`
		case "/page3":
			// Page 3: "apple banana cherry"
			pageContent = `<html><head><title>Page 3</title></head><body><p>apple banana cherry</p><a href="/page4">4</a></body></html>`
		case "/page4":
			// Page 4: "apple date date" (high TF for "date")
			pageContent = `<html><head><title>Page 4</title></head><body><p>apple date date</p><a href="/page5">5</a></body></html>`
		case "/page5":
			// Page 5: "grape" (unique term)
			pageContent = `<html><head><title>Page 5</title></head><body><p>grape</p></body></html>`
		default:
			http.NotFound(w, r)
			return
		}
		fmt.Fprintln(w, pageContent)
	}))
	defer ts.Close()

	// Setup a temporary database for the test.
	// os.Remove("test.db")
	tmpfile, err := os.CreateTemp("", "test-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp db file: %v", err)
	}
	dbPath := tmpfile.Name()
	tmpfile.Close()
	defer os.Remove(dbPath)

	// Initialize the indexer and crawler.
	indexer, err := newIndexer(dbPath)
	if err != nil {
		t.Fatalf("Failed to create indexer: %v", err)
	}
	defer indexer.db.Close()

	// Crawl a maximum of 5 pages for this test.
	crawler := NewCrawler(5, indexer)

	// Run the crawler.
	crawler.Start(ts.URL)

	// Query the indexer and verify the results against calculated TF-IDF scores.
	// N = 5 (total documents)
	// IDF = log10(N / df)
	const epsilon = 1e-4 // A small tolerance for floating-point comparisons.

	testCases := []struct {
		term     string
		expected map[string]float64 // Map from URL path to expected TF-IDF
	}{
		{
			"grape", // Unique term (df=1), highest IDF. TF is 1/1=1.
			map[string]float64{
				"/page5": (1.0 / 1.0) * math.Log10(5.0/1.0),
			},
		},
		{
			"date", // High TF (2/3), high IDF (df=1).
			map[string]float64{
				"/page4": (2.0 / 3.0) * math.Log10(5.0/1.0),
			},
		},
		{
			"banana", // Common term (df=2).
			map[string]float64{
				"":       (1.0 / 2.0) * math.Log10(5.0/2.0),
				"/page3": (1.0 / 3.0) * math.Log10(5.0/2.0),
			},
		},
		{
			"appl", // Very common term (df=4), low IDF.
			map[string]float64{
				"":       (1.0 / 2.0) * math.Log10(5.0/4.0),
				"/page2": (1.0 / 2.0) * math.Log10(5.0/4.0),
				"/page3": (1.0 / 3.0) * math.Log10(5.0/4.0),
				"/page4": (1.0 / 3.0) * math.Log10(5.0/4.0),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.term, func(t *testing.T) {
			results, err := indexer.queryResult(tc.term, 10)
			if err != nil {
				t.Fatalf("Query for term '%s' failed: %v", tc.term, err)
			}

			if len(results) != len(tc.expected) {
				t.Fatalf("Expected %d documents for term '%s', but got %d", len(tc.expected), tc.term, len(results))
			}

			// Create a map of actual results for easy lookup and verification.
			actualResults := make(map[string]float64)
			for _, res := range results {
				parsedURL, err := url.Parse(res.URL)
				if err != nil {
					t.Errorf("Could not parse result URL '%s': %v", res.URL, err)
					continue
				}
				actualResults[parsedURL.Path] = res.TfIdf
			}

			// Compare actual TF-IDF scores with expected scores.
			for path, expectedTfIdf := range tc.expected {
				actualTfIdf, ok := actualResults[path]
				if !ok {
					t.Errorf("Expected to find document '%s' for term '%s', but it was missing\n%v", path, tc.term, actualResults)
					continue
				}
				if math.Abs(actualTfIdf-expectedTfIdf) > epsilon {
					t.Errorf("Incorrect TF-IDF for term '%s' in doc '%s': got %f, want %f", tc.term, path, actualTfIdf, expectedTfIdf)
				}
			}
		})
	}
}
