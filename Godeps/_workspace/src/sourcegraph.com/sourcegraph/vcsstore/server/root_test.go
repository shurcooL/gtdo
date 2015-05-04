package server

import (
	"log"
	"net/http"
	"testing"

	"sourcegraph.com/sourcegraph/vcsstore/vcsclient"
)

func TestHandler_serveRoot(t *testing.T) {
	setupHandlerTest()
	defer teardownHandlerTest()

	resp, err := http.Get(server.URL + testHandler.router.URLTo(vcsclient.RouteRoot).String())
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	if got, want := resp.StatusCode, http.StatusOK; got != want {
		t.Errorf("got code %d, want %d", got, want)
	}
}
