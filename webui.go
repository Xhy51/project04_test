package main

import (
	"html/template"
	"log"
	"net/http"

	"github.com/kljensen/snowball/english"
)

// htmlTemplate defines the simple, style-less HTML for our UI.
// It uses Go template syntax to conditionally display search results.
const htmlTemplate = `
<!DOCTYPE html>
<html>
<head>
    <title>Go Search Engine</title>
</head>
<body>
    <h1>Search</h1>
    <form action="/search" method="get">
        <input type="text" name="q" value="{{.Query}}" size="60">
        <input type="submit" value="Search">
    </form>
    <hr>

    {{/* This block only renders if there are results */}}
    {{if .Results}}
        <h3>Results for "{{.Query}}"</h3>
        <table border="1" style="border-collapse: collapse; width: 100%;">
            <thead>
                <tr>
                    <th style="padding: 5px;">Count</th>
                    <th style="padding: 5px;">TF-IDF</th>
                    <th style="padding: 5px; text-align: left;">Title</th>
                </tr>
            </thead>
            <tbody>
                {{/* Loop over each search result */}}
                {{range .Results}}
                <tr>
                    <td style="padding: 5px;">{{.Count}}</td>
                    <td style="padding: 5px;">{{printf "%.4f" .TfIdf}}</td>
                    <td style="padding: 5px;">
                        <a href="{{.URL}}">{{.Title}}</a>
                    </td>
                </tr>
                {{end}}
            </tbody>
        </table>
    {{end}}
</body>
</html>
`

const MaxResults = 50

type templateData struct {
	Query   string
	Results []queryResult
}

type webUI struct {
	indexer  *Indexer
	template *template.Template
}

func newWebUI(indexer *Indexer) (*webUI, error) {
	tmpl, err := template.New("searchUI").Parse(htmlTemplate)
	if err != nil {
		return nil, err
	}
	return &webUI{
		indexer:  indexer,
		template: tmpl,
	}, nil
}

func (ui *webUI) handleSearch(w http.ResponseWriter, r *http.Request) {
	// Get the search query from the URL parameters (e.g., /search?q=hello)
	query := english.Stem(r.URL.Query().Get("q"), false)
	data := templateData{Query: query}

	// If the query is not empty, perform the search.
	if query != "" {
		results, err := ui.indexer.queryResult(query, MaxResults)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data.Results = results
	}

	// Execute the template, passing in the query and results.
	err := ui.template.Execute(w, data)
	if err != nil {
		// Log the error and send a generic error message to the user.
		log.Printf("Error executing template: %v", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}
