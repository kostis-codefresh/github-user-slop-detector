package main

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"time"
)

var startTime = time.Now()

type sysInfoRow struct {
	Label string
	Value string
}

// sysInfo reports metrics the Go runtime exposes about this process, plus
// WEBSITES_ENABLE_APP_SERVICE_STORAGE when set, since on Azure App Service it
// decides whether /home survives a restart.
func sysInfo() []sysInfoRow {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	rows := []sysInfoRow{
		{"Go version", runtime.Version()},
		{"OS / Arch", runtime.GOOS + "/" + runtime.GOARCH},
		{"PID", strconv.Itoa(os.Getpid())},
		{"Uptime", time.Since(startTime).Round(time.Second).String()},
		{"CPUs / GOMAXPROCS", fmt.Sprintf("%d / %d", runtime.NumCPU(), runtime.GOMAXPROCS(0))},
		{"Goroutines", strconv.Itoa(runtime.NumGoroutine())},
		{"Heap in use", formatBytes(m.HeapAlloc)},
		{"Memory from OS", formatBytes(m.Sys)},
	}
	if v, ok := os.LookupEnv("WEBSITES_ENABLE_APP_SERVICE_STORAGE"); ok {
		rows = append(rows, sysInfoRow{"WEBSITES_ENABLE_APP_SERVICE_STORAGE", v})
	}
	return rows
}

func formatBytes(b uint64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GiB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
