package server

import "net/http"

var (
	longCacheControl  = "max-age=31536000, public"
	shortCacheControl = "max-age=7, public"
)

func setLongCache(w http.ResponseWriter) {
	w.Header().Set("cache-control", longCacheControl)
}

func setShortCache(w http.ResponseWriter) {
	w.Header().Set("cache-control", shortCacheControl)
}
