// contributor-report-web is the web front end for the contributor report. For
// now it serves only the home page; the Analyze button is inert until the
// server has a GitHub token to fetch with.
package main

import (
	"log"
	"net/http"
	"time"

	"github.com/kostis-codefresh/github-user-slop-detector/ui"
)

func newHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, ui.Embedded, "index.html")
	})
	return mux
}

func main() {
	srv := &http.Server{
		Addr:              ":8080",
		Handler:           newHandler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("listening on %s", srv.Addr)
	log.Fatal(srv.ListenAndServe())
}
