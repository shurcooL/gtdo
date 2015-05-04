package server

import "net/http"

func (h *Handler) serveRoot(w http.ResponseWriter, r *http.Request) error {
	w.Write([]byte("vcsstore"))
	return nil
}
