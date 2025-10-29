package main

import (
	"database/sql"
	"fmt"
	"log"
	"math"
	"sync"

	_ "modernc.org/sqlite"
)

type Indexer struct {
	mu sync.Mutex
	db *sql.DB
}

func (i *Indexer) addPage(page Page) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	// Start a transaction for data consistency.
	tx, err := i.db.Begin()
	if err != nil {
		return fmt.Errorf("could not begin transaction: %w", err)
	}
	// Rollback the transaction if anything fails.
	defer tx.Rollback()

	// Insert the page
	res, err := tx.Exec("INSERT INTO pages(url, title) VALUES(?, ?)", page.Url, page.Title)
	if err != nil {
		return fmt.Errorf("could not insert page: %w", err)
	}
	pageID, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("could not get last insert ID for page: %w", err)
	}

	// Prepare for batch inserting terms. "INSERT OR IGNORE" handles duplicates.
	termStmt, err := tx.Prepare("INSERT OR IGNORE INTO terms(term) VALUES(?)")
	if err != nil {
		return err
	}
	defer termStmt.Close()

	// Prepare for batch inserting hits. "INSERT OR IGNORE" handles duplicates.
	hitStmt, err := tx.Prepare(`
		INSERT INTO hits(page_id, term_id, count, freq)
		VALUES(?, (SELECT id FROM terms WHERE term = ?), ?, ?)
	`)
	if err != nil {
		return err
	}
	defer hitStmt.Close()

	// Batch insertion.
	for term, count := range page.TermCounts {
		freq := page.TermFreq[term]

		if _, err := termStmt.Exec(term); err != nil {
			return fmt.Errorf("could not insert term '%s': %w", term, err)
		}

		if _, err := hitStmt.Exec(pageID, term, count, freq); err != nil {
			return fmt.Errorf("could not insert hit for term '%s': %w", term, err)
		}
	}

	// Commit the transaction.
	log.Printf("Indexed page: %s", page.Url)
	return tx.Commit()
}

type queryResult struct {
	Title string
	URL   string
	Count int
	TfIdf float64
}

func (i *Indexer) queryResult(term string, limit int) ([]queryResult, error) {
	querySQL := `
	WITH Constants AS (
		SELECT
			LOG(
				(SELECT CAST(COUNT(*) AS REAL) FROM pages) /
				MAX(1, (SELECT CAST(COUNT(*) AS REAL) FROM hits h JOIN terms t ON h.term_id = t.id WHERE t.term = ?))
			) AS idf
	)
	SELECT
		p.title,
		p.url,
		h.count,
		h.freq * c.idf AS tfidf
	FROM
		pages p
	JOIN
		hits h ON p.id = h.page_id
	JOIN
		terms t ON h.term_id = t.id
	CROSS JOIN
		Constants c
	WHERE
		t.term = ?
	ORDER BY
		tfidf DESC, p.url ASC
	`
	if limit > 0 {
		querySQL += fmt.Sprintf("LIMIT %d", limit)
	}

	i.mu.Lock()
	rows, err := i.db.Query(querySQL, term, term)
	i.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("database query failed: %w", err)
	}
	defer rows.Close()

	var results []queryResult
	for rows.Next() {
		var res queryResult
		if err := rows.Scan(&res.Title, &res.URL, &res.Count, &res.TfIdf); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		// Handle potential NaN or Inf values from LOG(0) if df > N (should not happen).
		if math.IsNaN(res.TfIdf) || math.IsInf(res.TfIdf, 0) {
			res.TfIdf = 0
		}
		results = append(results, res)
	}

	return results, nil
}

func (i *Indexer) queryUrlVisited(u string) (bool, error) {
	i.mu.Lock()
	rows, err := i.db.Query("SELECT COUNT(*) FROM pages WHERE url = ?", u)
	i.mu.Unlock()
	if err != nil {
		return false, fmt.Errorf("failed to query URL: %w", err)
	}
	defer rows.Close()

	var count int
	if rows.Next() {
		err = rows.Scan(&count)
		if err != nil {
			return false, fmt.Errorf("failed to scan count: %w", err)
		}
	}

	return count > 0, nil
}

func newIndexer(path string) (*Indexer, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec("PRAGMA foreign_keys = ON;")
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign key: %w", err)
	}
	_, err = db.Exec("PRAGMA journal_mode = MEMORY;")
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to set journal mode: %w", err)
	}

	createPagesSQL := `
	CREATE TABLE IF NOT EXISTS pages (
		id INTEGER NOT NULL PRIMARY KEY,
		url TEXT NOT NULL UNIQUE,
		title TEXT NOT NULL
	);`
	if _, err := db.Exec(createPagesSQL); err != nil {
		return nil, fmt.Errorf("failed to create pages table: %w", err)
	}

	createTermsSQL := `
	CREATE TABLE IF NOT EXISTS terms (
		id INTEGER NOT NULL PRIMARY KEY,
		term TEXT NOT NULL UNIQUE
	);`
	if _, err := db.Exec(createTermsSQL); err != nil {
		return nil, fmt.Errorf("failed to create terms table: %w", err)
	}

	createHitsSQL := `
	CREATE TABLE IF NOT EXISTS hits (
		page_id INTEGER NOT NULL,
		term_id INTEGER NOT NULL,
		count INTEGER NOT NULL,
		freq REAL NOT NULL,
		PRIMARY KEY(page_id, term_id),
		FOREIGN KEY(page_id) REFERENCES pages(id),
		FOREIGN KEY(term_id) REFERENCES terms(id)
	);`
	if _, err := db.Exec(createHitsSQL); err != nil {
		return nil, fmt.Errorf("failed to create hits table: %w", err)
	}

	s := &Indexer{db: db}

	return s, nil
}
