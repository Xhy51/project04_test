package main

import "golang.org/x/net/html"

type Response struct {
	Url      string
	HtmlRoot *html.Node
}

type Page struct {
	Title         string
	Url           string
	TermCounts    map[string]int
	TermFreq      map[string]float64
	ContainedUrls []string
}
