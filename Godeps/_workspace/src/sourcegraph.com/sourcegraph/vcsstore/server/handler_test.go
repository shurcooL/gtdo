package server

import (
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

var (
	server      *httptest.Server
	testHandler *Handler
)

func setupHandlerTest() {
	testHandler = NewHandler(nil, nil, nil)
	server = httptest.NewServer(testHandler)
}

func teardownHandlerTest() {
	server.Close()
	testHandler = nil
}

func testRedirectedTo(t *testing.T, resp *http.Response, status int, wantLocation *url.URL) {
	if got, want := resp.StatusCode, status; got != want {
		t.Errorf("got redirection code %d, want %d", got, want)
	}
	if status >= 300 && status < 400 {
		if location := resp.Header.Get("location"); location != wantLocation.String() {
			t.Errorf("got redirection Location: %s, want %s", location, wantLocation)
		}
	}
}

func logResponseBody(t *testing.T, r *http.Response) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("HTTP response body was: %q", body)
}

var (
	errIgnoredRedirect    = errors.New("not following redirect")
	ignoreRedirectsClient = http.Client{
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			return errIgnoredRedirect
		},
	}
)

func isIgnoredRedirectErr(err error) bool {
	if err, ok := err.(*url.Error); ok && err.Err == errIgnoredRedirect {
		return true
	}
	return false
}
