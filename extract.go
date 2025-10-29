package main

import (
	"log"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/kljensen/snowball/english"
	"golang.org/x/net/html"
)

func walk(n *html.Node, page *Page, ignoreText bool) {
	// Regex to find english words, including "foo", "foo2", "foo-bar" and "foo_bar"
	var wordRegex = regexp.MustCompile(`(?i)[a-z][a-z0-9\-_]*`)
	if !ignoreText {
		// Extract words from text node
		if n.Type == html.TextNode {
			for _, word := range wordRegex.FindAllString(n.Data, -1) {
				word = english.Stem(word, false)
				if english.IsStopWord(word) {
					continue
				}
				if c, ok := page.TermCounts[word]; ok {
					page.TermCounts[word] = c + 1
				} else {
					page.TermCounts[word] = 1
				}
			}
		}
	}

	// Extract title from <title> element
	if n.Type == html.ElementNode && strings.EqualFold(n.Data, "title") {
		if strings.EqualFold(n.Parent.Data, "head") {
			if n.FirstChild != nil {
				page.Title = strings.TrimSpace(n.FirstChild.Data)
			}
		}
	}

	// Extract urls from <a> element
	if n.Type == html.ElementNode && strings.EqualFold(n.Data, "a") {
		for _, attr := range n.Attr {
			if strings.EqualFold(attr.Key, "href") {
				page.ContainedUrls = append(page.ContainedUrls, attr.Val)
			}
		}
	}

	if n.Type == html.ElementNode && strings.EqualFold(n.Data, "body") {
		ignoreText = false
	}
	// Recursively walk the html tree
	for c := range n.ChildNodes() {
		walk(c, page, ignoreText)
	}
}

func extracter(wg *sync.WaitGroup, responses <-chan Response, pages chan<- Page) {
	defer wg.Done()
	for resp := range responses {
		page := Page{Title: "No Title", Url: resp.Url, TermCounts: make(map[string]int), TermFreq: make(map[string]float64), ContainedUrls: nil}
		// Walk to collect terms and urls
		walk(resp.HtmlRoot, &page, true)
		// Calculate TF (Term Frequency)
		totalTerms := 0
		for _, count := range page.TermCounts {
			totalTerms += count
		}
		for term, count := range page.TermCounts {
			page.TermFreq[term] = float64(count) / float64(totalTerms)
		}
		// Convert to absolute URLs
		base, _ := url.Parse(resp.Url)
		urls := make([]string, 0)
		for _, href := range page.ContainedUrls {
			u, err := url.Parse(href)
			if err != nil {
				continue
			}
			// Drop query parameters and fragment
			u.RawQuery = ""
			u.Fragment = ""
			absURL := base.ResolveReference(u).String()
			urls = append(urls, absURL)
		}
		page.ContainedUrls = urls
		// Send over channel
		log.Printf("Extracted %s\n", resp.Url)
		pages <- page
	}
}
