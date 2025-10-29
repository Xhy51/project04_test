package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const UrlBufferSize = 1 << 20
const ResponseBufferSize = 16

type rep struct {
	Host      string
	Disallows []string
}

func (r *rep) allow(url_ string) (bool, error) {
	u, err := url.Parse(url_)
	if err != nil {
		return false, err
	}
	if u.Host != r.Host {
		return false, fmt.Errorf("unmatched REP host: expected \"%s\", got \"%s\"", r.Host, u.Host)
	}
	for _, rule := range r.Disallows {
		if strings.HasPrefix(u.Path, rule) {
			return false, nil
		}
	}
	return true, nil
}

type dispatcher struct {
	dispatchMu  sync.Mutex
	reps        map[string]rep
	urlsToFetch map[string]chan string
	dispatched  map[string]struct{}
	responses   chan Response
	wg          sync.WaitGroup
}

func newDispatcher() *dispatcher {
	return &dispatcher{
		reps:        make(map[string]rep),
		urlsToFetch: make(map[string]chan string),
		dispatched:  make(map[string]struct{}),
		responses:   make(chan Response, ResponseBufferSize),
	}
}

func newREP(host string, robotsTxt io.Reader) rep {
	r := rep{host, nil}
	if robotsTxt == nil {
		return r
	}
	scanner := bufio.NewScanner(robotsTxt)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ":")
		if len(parts) == 2 && strings.ToLower(strings.TrimSpace(parts[0])) == "disallow" {
			if prefix := strings.TrimSpace(parts[1]); prefix != "" {
				r.Disallows = append(r.Disallows, prefix)
			}
		}
	}
	return r
}

func fetchREP(host string, scheme string) (rep, error) {
	resp, err := http.Get(fmt.Sprintf("%s://%s/robots.txt", scheme, host))
	if err != nil {
		return rep{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return newREP(host, resp.Body), nil
	} else {
		return newREP(host, nil), nil
	}
}

func (d *dispatcher) getREP(host string, scheme string) (rep, error) {
	if r, ok := d.reps[host]; ok {
		return r, nil
	} else {
		r, err := fetchREP(host, scheme)
		if err == nil {
			d.reps[host] = r
		}
		return r, err
	}
}

func (d *dispatcher) dispatch(url_ string) (bool, error) {
	u, err := url.Parse(url_)
	if err != nil {
		return false, err
	}

	d.dispatchMu.Lock()

	if _, ok := d.dispatched[url_]; ok {
		d.dispatchMu.Unlock()
		return false, nil
	}

	// Get Robot Exclusion Protocol
	r, err := d.getREP(u.Host, u.Scheme)
	// Ignore robots.txt on fetch failure
	if err != nil {
		r = newREP(u.Host, nil)
	}
	// Allocate fetcher
	urls, ok := d.urlsToFetch[u.Host]
	if !ok {
		urls = make(chan string, UrlBufferSize)
		d.urlsToFetch[u.Host] = urls
		d.wg.Add(1)
		go fetcher(&d.wg, u.Host, urls, d.responses)
	}

	d.dispatched[url_] = struct{}{}

	d.dispatchMu.Unlock()

	if allowed, err := r.allow(url_); err == nil && allowed {
		urls <- url_
	}
	return true, nil
}

func (d *dispatcher) close() {
	d.dispatchMu.Lock()
	for _, urls := range d.urlsToFetch {
		close(urls)
	}
	d.dispatchMu.Unlock()
}

func (d *dispatcher) wait() {
	d.wg.Wait()
	close(d.responses)
}
