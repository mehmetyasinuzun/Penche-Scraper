package api

import (
	_ "embed"
	"net/http"
)

//go:embed gallery.html
var galleryHTML []byte

// serveGallery returns the static gallery shell. All data is fetched
// client-side from the JSON API: GET /v1/stats, GET /v1/events, and
// GET /v1/events/{id}/image. Keeping the page static means the initial
// response is tiny and screenshots stream lazily instead of being
// base64-inlined into one huge document.
func serveGallery(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(galleryHTML)
}
