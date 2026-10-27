package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/kostis-codefresh/github-user-slop-detector/pkg/metrics"
)

// maxReports bounds the in-memory cache, and with it the home page's list of
// previous reports. The file is not bounded.
const maxReports = 20

// reportCache keeps evaluated reports in memory, keyed by lower-cased login
// because GitHub logins are case-insensitive, and also in one JSON file shared
// by every visitor, so reports survive a restart (on Azure App Service /home
// is the persistent part of the disk). It holds metrics.Report rather than
// rendered HTML, so a cached report goes through the same renderer as a fresh
// one. An empty path keeps it memory-only.
type reportCache struct {
	mu      sync.Mutex
	path    string
	reports map[string]metrics.Report
}

// newReportCache seeds memory with the newest reports from the file, so the
// home page lists them after a restart.
func newReportCache(path string) *reportCache {
	c := &reportCache{path: path, reports: make(map[string]metrics.Report)}
	stored := c.readFile()
	all := make([]metrics.Report, 0, len(stored))
	for _, r := range stored {
		all = append(all, r)
	}
	slices.SortFunc(all, func(a, b metrics.Report) int { return b.WindowEnd.Compare(a.WindowEnd) })
	for _, r := range all[:min(len(all), maxReports)] {
		c.reports[strings.ToLower(r.Username)] = r
	}
	return c
}

// get checks memory first, then the file, which still holds reports that
// memory has evicted.
func (c *reportCache) get(login string) (metrics.Report, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := strings.ToLower(login)
	if r, ok := c.reports[key]; ok {
		return r, true
	}
	r, ok := c.readFile()[key]
	if ok {
		c.putLocked(key, r)
	}
	return r, ok
}

// add stores r in memory and in the file. A file that cannot be written is
// only logged: the report is still served from memory.
func (c *reportCache) add(r metrics.Report) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := strings.ToLower(r.Username)
	c.putLocked(key, r)
	if c.path == "" {
		return
	}
	stored := c.readFile()
	stored[key] = r
	if err := c.writeFile(stored); err != nil {
		log.Printf("writing report cache %s: %v", c.path, err)
	}
}

// putLocked stores r in memory, dropping the oldest report when it is full.
// WindowEnd is when the report's data was fetched, so "oldest" is the one
// created first.
func (c *reportCache) putLocked(key string, r metrics.Report) {
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

// readFile returns every report in the file. A missing file is an empty
// cache; an unreadable one is logged and treated as empty too.
func (c *reportCache) readFile() map[string]metrics.Report {
	stored := make(map[string]metrics.Report)
	if c.path == "" {
		return stored
	}
	data, err := os.ReadFile(c.path)
	if errors.Is(err, fs.ErrNotExist) {
		return stored
	}
	if err == nil {
		err = json.Unmarshal(data, &stored)
	}
	if err != nil {
		log.Printf("reading report cache %s: %v", c.path, err)
		return make(map[string]metrics.Report)
	}
	return stored
}

// writeFile replaces the file through a rename, so a crash mid-write cannot
// leave it truncated.
func (c *reportCache) writeFile(stored map[string]metrics.Report) error {
	data, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}

// list returns the reports in memory, newest first.
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
