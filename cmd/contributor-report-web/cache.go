package main

import (
	"slices"
	"strings"
	"sync"

	"github.com/kostis-codefresh/github-user-slop-detector/pkg/metrics"
)

// maxReports bounds the cache, and with it the home page's list of previous
// reports.
const maxReports = 20

// reportCache keeps evaluated reports in memory, keyed by lower-cased login
// because GitHub logins are case-insensitive. It holds metrics.Report rather
// than rendered HTML, so a cached report goes through the same renderer as a
// fresh one. It is lost on restart.
type reportCache struct {
	mu      sync.Mutex
	reports map[string]metrics.Report
}

func newReportCache() *reportCache {
	return &reportCache{reports: make(map[string]metrics.Report)}
}

func (c *reportCache) get(login string) (metrics.Report, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.reports[strings.ToLower(login)]
	return r, ok
}

// add stores r, dropping the oldest report when the cache is full. WindowEnd
// is when the report's data was fetched, so "oldest" is the one created first.
func (c *reportCache) add(r metrics.Report) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := strings.ToLower(r.Username)
	if _, ok := c.reports[key]; !ok && len(c.reports) >= maxReports {
		var oldest string
		for k, v := range c.reports {
			if oldest == "" || v.WindowEnd.Before(c.reports[oldest].WindowEnd) {
				oldest = k
			}
		}
		delete(c.reports, oldest)
	}
	c.reports[key] = r
}

// list returns the cached reports, newest first.
func (c *reportCache) list() []metrics.Report {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]metrics.Report, 0, len(c.reports))
	for _, r := range c.reports {
		out = append(out, r)
	}
	slices.SortFunc(out, func(a, b metrics.Report) int { return b.WindowEnd.Compare(a.WindowEnd) })
	return out
}
