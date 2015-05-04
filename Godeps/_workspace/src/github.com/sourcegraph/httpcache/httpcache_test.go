package httpcache

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

type S struct {
	server    *httptest.Server
	client    http.Client
	transport *Transport
}

var s S

type fakeClock struct {
	elapsed time.Duration
}

func (c *fakeClock) since(t time.Time) time.Duration {
	return c.elapsed
}

func setup() {
	s = S{}
	tp := NewMemoryCacheTransport()
	client := http.Client{Transport: tp}
	s.transport = tp
	s.client = client

	mux := http.NewServeMux()
	s.server = httptest.NewServer(mux)

	mux.HandleFunc("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
	}))

	mux.HandleFunc("/nostore", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
	}))

	mux.HandleFunc("/etag", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		etag := "124567"
		if r.Header.Get("if-none-match") == etag {
			w.WriteHeader(http.StatusNotModified)
		}
		w.Header().Set("etag", etag)
	}))

	mux.HandleFunc("/lastmodified", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lm := "Fri, 14 Dec 2010 01:01:50 GMT"
		if r.Header.Get("if-modified-since") == lm {
			w.WriteHeader(http.StatusNotModified)
		}
		w.Header().Set("last-modified", lm)
	}))

	mux.HandleFunc("/varyaccept", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Vary", "Accept")
		w.Write([]byte("Some text content"))
	}))

	mux.HandleFunc("/doublevary", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Vary", "Accept, Accept-Language")
		w.Write([]byte("Some text content"))
	}))
	mux.HandleFunc("/2varyheaders", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Add("Vary", "Accept")
		w.Header().Add("Vary", "Accept-Language")
		w.Write([]byte("Some text content"))
	}))
	mux.HandleFunc("/varyunused", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Vary", "X-Madeup-Header")
		w.Write([]byte("Some text content"))
	}))

	updateFieldsCounter := 0
	mux.HandleFunc("/updatefields", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Counter", strconv.Itoa(updateFieldsCounter))
		w.Header().Set("Etag", `"e"`)
		updateFieldsCounter++
		if r.Header.Get("if-none-match") != "" {
			w.WriteHeader(http.StatusNotModified)
		} else {
			w.Write([]byte("Some text content"))
		}
	}))
}

func tearDownTest() {
	s.transport.Cache = NewMemoryCache()
	clock = &realClock{}
	s.server.Close()
}

func TestGetOnlyIfCachedHit(t *testing.T) {
	setup()
	defer tearDownTest()
	req, err := http.NewRequest("GET", s.server.URL, nil)
	if err != nil {
		t.FailNow()
	}
	resp, err := s.client.Do(req)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer resp.Body.Close()
	if resp.Header.Get(XFromCache) != "" {
		t.FailNow()
	}

	req2, err2 := http.NewRequest("GET", s.server.URL, nil)
	req2.Header.Add("cache-control", "only-if-cached")
	resp2, err2 := s.client.Do(req)
	defer resp2.Body.Close()
	if err2 != nil || resp2.Header.Get(XFromCache) != "1" || resp2.StatusCode != http.StatusOK {
		t.FailNow()
	}
}

func TestGetOnlyIfCachedMiss(t *testing.T) {
	setup()
	defer tearDownTest()
	req, err := http.NewRequest("GET", s.server.URL, nil)
	req.Header.Add("cache-control", "only-if-cached")
	resp, err := s.client.Do(req)
	defer resp.Body.Close()
	if err != nil || resp.Header.Get(XFromCache) != "" || resp.StatusCode != 504 {
		t.FailNow()
	}
}

func TestGetNoStoreRequest(t *testing.T) {
	setup()
	defer tearDownTest()
	req, err := http.NewRequest("GET", s.server.URL, nil)
	req.Header.Add("Cache-Control", "no-store")
	resp, err := s.client.Do(req)
	defer resp.Body.Close()
	if err != nil || resp.Header.Get(XFromCache) != "" {
		t.FailNow()
	}

	resp2, err2 := s.client.Do(req)
	defer resp2.Body.Close()
	if err2 != nil || resp2.Header.Get(XFromCache) != "" {
		t.FailNow()
	}
}

func TestGetNoStoreResponse(t *testing.T) {
	setup()
	defer tearDownTest()
	req, err := http.NewRequest("GET", s.server.URL+"/nostore", nil)
	resp, err := s.client.Do(req)
	defer resp.Body.Close()
	if err != nil || resp.Header.Get(XFromCache) != "" {
		t.FailNow()
	}

	resp2, err2 := s.client.Do(req)
	defer resp2.Body.Close()
	if err2 != nil || resp2.Header.Get(XFromCache) != "" {
		t.FailNow()
	}
}

func TestGetWithEtag(t *testing.T) {
	setup()
	defer tearDownTest()
	req, err := http.NewRequest("GET", s.server.URL+"/etag", nil)
	resp, err := s.client.Do(req)
	defer resp.Body.Close()
	if err != nil || resp.Header.Get(XFromCache) != "" {
		t.FailNow()
	}

	resp2, err2 := s.client.Do(req)
	defer resp2.Body.Close()
	if err2 != nil || resp2.Header.Get(XFromCache) != "1" {
		t.FailNow()
	}
	// additional assertions to verify that 304 response is converted properly
	if resp2.Status != "200 OK" {
		t.FailNow()
	}

	_, ok := resp2.Header["Connection"]
	if ok {
		t.FailNow()
	}
}

func TestGetWithLastModified(t *testing.T) {
	setup()
	defer tearDownTest()
	req, err := http.NewRequest("GET", s.server.URL+"/lastmodified", nil)
	resp, err := s.client.Do(req)
	defer resp.Body.Close()
	if err != nil || resp.Header.Get(XFromCache) != "" {
		t.FailNow()
	}

	resp2, err2 := s.client.Do(req)
	defer resp2.Body.Close()
	if err2 != nil || resp2.Header.Get(XFromCache) != "1" {
		t.FailNow()
	}
}

func TestGetWithVary(t *testing.T) {
	setup()
	defer tearDownTest()
	req, err := http.NewRequest("GET", s.server.URL+"/varyaccept", nil)
	req.Header.Set("Accept", "text/plain")
	resp, err := s.client.Do(req)
	defer resp.Body.Close()
	if err != nil || resp.Header.Get("Vary") != "Accept" {
		t.FailNow()
	}

	resp2, err2 := s.client.Do(req)
	defer resp2.Body.Close()
	if err2 != nil || resp2.Header.Get(XFromCache) != "1" {
		t.FailNow()
	}

	req.Header.Set("Accept", "text/html")
	resp3, err3 := s.client.Do(req)
	defer resp3.Body.Close()
	if err3 != nil || resp3.Header.Get(XFromCache) != "" {
		t.FailNow()
	}

	req.Header.Set("Accept", "")
	resp4, err4 := s.client.Do(req)
	defer resp4.Body.Close()
	if err4 != nil || resp4.Header.Get(XFromCache) != "" {
		t.FailNow()
	}
}

func TestGetWithDoubleVary(t *testing.T) {
	setup()
	defer tearDownTest()
	req, err := http.NewRequest("GET", s.server.URL+"/doublevary", nil)
	req.Header.Set("Accept", "text/plain")
	req.Header.Set("Accept-Language", "da, en-gb;q=0.8, en;q=0.7")
	resp, err := s.client.Do(req)
	defer resp.Body.Close()
	if err != nil || resp.Header.Get("Vary") == "" {
		t.FailNow()
	}

	resp2, err2 := s.client.Do(req)
	defer resp2.Body.Close()
	if err2 != nil || resp2.Header.Get(XFromCache) != "1" {
		t.FailNow()
	}

	req.Header.Set("Accept-Language", "")
	resp3, err3 := s.client.Do(req)
	defer resp3.Body.Close()
	if err3 != nil || resp3.Header.Get(XFromCache) != "" {
		t.FailNow()
	}

	req.Header.Set("Accept-Language", "da")
	resp4, err4 := s.client.Do(req)
	defer resp4.Body.Close()
	if err4 != nil || resp4.Header.Get(XFromCache) != "" {
		t.FailNow()
	}
}

func TestGetWith2VaryHeaders(t *testing.T) {
	setup()
	defer tearDownTest()
	// Tests that multiple Vary headers' comma-separated lists are
	// merged. See https://github.com/gregjones/httpcache/issues/27.
	const (
		accept         = "text/plain"
		acceptLanguage = "da, en-gb;q=0.8, en;q=0.7"
	)
	req, err := http.NewRequest("GET", s.server.URL+"/2varyheaders", nil)
	req.Header.Set("Accept", accept)
	req.Header.Set("Accept-Language", acceptLanguage)
	resp, err := s.client.Do(req)
	defer resp.Body.Close()
	if err != nil || resp.Header.Get("Vary") == "" {
		t.FailNow()
	}

	resp2, err2 := s.client.Do(req)
	defer resp2.Body.Close()
	if err2 != nil || resp2.Header.Get(XFromCache) != "1" {
		t.FailNow()
	}

	req.Header.Set("Accept-Language", "")
	resp3, err3 := s.client.Do(req)
	defer resp3.Body.Close()
	if err3 != nil || resp3.Header.Get(XFromCache) != "" {
		t.FailNow()
	}

	req.Header.Set("Accept-Language", "da")
	resp4, err4 := s.client.Do(req)
	defer resp4.Body.Close()
	if err4 != nil || resp4.Header.Get(XFromCache) != "" {
		t.FailNow()
	}

	req.Header.Set("Accept-Language", acceptLanguage)
	req.Header.Set("Accept", "")
	resp5, err5 := s.client.Do(req)
	defer resp5.Body.Close()
	if err5 != nil || resp5.Header.Get(XFromCache) != "" {
		t.FailNow()
	}

	req.Header.Set("Accept", "image/png")
	resp6, err6 := s.client.Do(req)
	defer resp6.Body.Close()
	if err6 != nil || resp6.Header.Get(XFromCache) != "" {
		t.FailNow()
	}

	resp7, err7 := s.client.Do(req)
	defer resp7.Body.Close()
	if err7 != nil || resp7.Header.Get(XFromCache) != "1" {
		t.FailNow()
	}
}

func TestGetVaryUnused(t *testing.T) {
	setup()
	defer tearDownTest()
	req, err := http.NewRequest("GET", s.server.URL+"/varyunused", nil)
	req.Header.Set("Accept", "text/plain")
	resp, err := s.client.Do(req)
	defer resp.Body.Close()
	if err != nil || resp.Header.Get("Vary") == "" {
		t.FailNow()
	}

	resp2, err2 := s.client.Do(req)
	defer resp2.Body.Close()
	if err2 != nil || resp2.Header.Get(XFromCache) != "1" {
		t.FailNow()
	}
}

func TestUpdateFields(t *testing.T) {
	setup()
	defer tearDownTest()
	req, err := http.NewRequest("GET", s.server.URL+"/updatefields", nil)
	resp, err := s.client.Do(req)
	defer resp.Body.Close()
	if err != nil {
		t.FailNow()
	}
	counter := resp.Header.Get("x-counter")

	resp2, err2 := s.client.Do(req)
	defer resp2.Body.Close()
	if err2 != nil || resp2.Header.Get(XFromCache) != "1" {
		t.FailNow()
	}
	counter2 := resp2.Header.Get("x-counter")

	if counter == counter2 {
		t.FailNow()
	}
}

func TestParseCacheControl(t *testing.T) {
	setup()
	defer tearDownTest()
	h := http.Header{}
	for _ = range parseCacheControl(h) {
		t.Fatal("cacheControl should be empty")
	}

	h.Set("cache-control", "no-cache")
	cc := parseCacheControl(h)
	if _, ok := cc["foo"]; ok {
		t.Error("Value shouldn't exist")
	}
	if nocache, ok := cc["no-cache"]; ok {
		if nocache != "" {
			t.FailNow()
		}
	}

	h.Set("cache-control", "no-cache, max-age=3600")
	cc = parseCacheControl(h)
	if cc["no-cache"] != "" || cc["max-age"] != "3600" {
		t.FailNow()
	}
}

func TestNoCacheRequestExpiration(t *testing.T) {
	setup()
	defer tearDownTest()
	respHeaders := http.Header{}
	respHeaders.Set("Cache-Control", "max-age=7200")
	reqHeaders := http.Header{}
	reqHeaders.Set("Cache-Control", "no-cache")

	if getFreshness(respHeaders, reqHeaders) != transparent {
		t.FailNow()
	}
}

func TestNoCacheResponseExpiration(t *testing.T) {
	setup()
	defer tearDownTest()
	respHeaders := http.Header{}
	respHeaders.Set("Cache-Control", "no-cache")
	respHeaders.Set("Expires", "Wed, 19 Apr 3000 11:43:00 GMT")
	reqHeaders := http.Header{}

	if getFreshness(respHeaders, reqHeaders) != stale {
		t.FailNow()
	}
}

func TestReqMustRevalidate(t *testing.T) {
	setup()
	defer tearDownTest()
	// not paying attention to request setting max-stale means never returning stale
	// responses, so always acting as if must-revalidate is set
	respHeaders := http.Header{}
	reqHeaders := http.Header{}
	reqHeaders.Set("Cache-Control", "must-revalidate")

	if getFreshness(respHeaders, reqHeaders) != stale {
		t.FailNow()
	}
}

func TestRespMustRevalidate(t *testing.T) {
	setup()
	defer tearDownTest()
	respHeaders := http.Header{}
	respHeaders.Set("Cache-Control", "must-revalidate")
	reqHeaders := http.Header{}

	if getFreshness(respHeaders, reqHeaders) != stale {
		t.FailNow()
	}
}

func TestFreshExpiration(t *testing.T) {
	setup()
	defer tearDownTest()
	now := time.Now()
	respHeaders := http.Header{}
	respHeaders.Set("date", now.Format(time.RFC1123))
	respHeaders.Set("expires", now.Add(time.Duration(2)*time.Second).Format(time.RFC1123))

	reqHeaders := http.Header{}
	if getFreshness(respHeaders, reqHeaders) != fresh {
		t.FailNow()
	}

	clock = &fakeClock{elapsed: 3 * time.Second}
	if getFreshness(respHeaders, reqHeaders) != stale {
		t.FailNow()
	}
}

func TestMaxAge(t *testing.T) {
	setup()
	defer tearDownTest()
	now := time.Now()
	respHeaders := http.Header{}
	respHeaders.Set("date", now.Format(time.RFC1123))
	respHeaders.Set("cache-control", "max-age=2")

	reqHeaders := http.Header{}
	if getFreshness(respHeaders, reqHeaders) != fresh {
		t.FailNow()
	}

	clock = &fakeClock{elapsed: 3 * time.Second}
	if getFreshness(respHeaders, reqHeaders) != stale {
		t.FailNow()
	}
}

func TestMaxAgeZero(t *testing.T) {
	setup()
	defer tearDownTest()
	now := time.Now()
	respHeaders := http.Header{}
	respHeaders.Set("date", now.Format(time.RFC1123))
	respHeaders.Set("cache-control", "max-age=0")

	reqHeaders := http.Header{}
	if getFreshness(respHeaders, reqHeaders) != stale {
		t.FailNow()
	}
}

func TestBothMaxAge(t *testing.T) {
	setup()
	defer tearDownTest()
	now := time.Now()
	respHeaders := http.Header{}
	respHeaders.Set("date", now.Format(time.RFC1123))
	respHeaders.Set("cache-control", "max-age=2")

	reqHeaders := http.Header{}
	reqHeaders.Set("cache-control", "max-age=0")
	if getFreshness(respHeaders, reqHeaders) != stale {
		t.FailNow()
	}
}

func TestMinFreshWithExpires(t *testing.T) {
	setup()
	defer tearDownTest()
	now := time.Now()
	respHeaders := http.Header{}
	respHeaders.Set("date", now.Format(time.RFC1123))
	respHeaders.Set("expires", now.Add(time.Duration(2)*time.Second).Format(time.RFC1123))

	reqHeaders := http.Header{}
	reqHeaders.Set("cache-control", "min-fresh=1")
	if getFreshness(respHeaders, reqHeaders) != fresh {
		t.FailNow()
	}

	reqHeaders = http.Header{}
	reqHeaders.Set("cache-control", "min-fresh=2")
	if getFreshness(respHeaders, reqHeaders) != stale {
		t.FailNow()
	}
}

func TestEmptyMaxStale(t *testing.T) {
	setup()
	defer tearDownTest()
	now := time.Now()
	respHeaders := http.Header{}
	respHeaders.Set("date", now.Format(time.RFC1123))
	respHeaders.Set("cache-control", "max-age=20")

	reqHeaders := http.Header{}
	reqHeaders.Set("cache-control", "max-stale")

	clock = &fakeClock{elapsed: 10 * time.Second}

	if getFreshness(respHeaders, reqHeaders) != fresh {
		t.FailNow()
	}

	clock = &fakeClock{elapsed: 60 * time.Second}

	if getFreshness(respHeaders, reqHeaders) != fresh {
		t.FailNow()
	}
}

func TestMaxStaleValue(t *testing.T) {
	setup()
	defer tearDownTest()
	now := time.Now()
	respHeaders := http.Header{}
	respHeaders.Set("date", now.Format(time.RFC1123))
	respHeaders.Set("cache-control", "max-age=10")

	reqHeaders := http.Header{}
	reqHeaders.Set("cache-control", "max-stale=20")
	clock = &fakeClock{elapsed: 5 * time.Second}

	if getFreshness(respHeaders, reqHeaders) != fresh {
		t.FailNow()
	}

	clock = &fakeClock{elapsed: 15 * time.Second}

	if getFreshness(respHeaders, reqHeaders) != fresh {
		t.FailNow()
	}

	clock = &fakeClock{elapsed: 30 * time.Second}

	if getFreshness(respHeaders, reqHeaders) != stale {
		t.FailNow()
	}
}

func containsHeader(headers []string, header string) bool {
	for _, v := range headers {
		if http.CanonicalHeaderKey(v) == http.CanonicalHeaderKey(header) {
			return true
		}
	}
	return false
}

func TestGetEndToEndHeaders(t *testing.T) {
	setup()
	var (
		headers http.Header
		end2end []string
	)

	headers = http.Header{}
	headers.Set("content-type", "text/html")
	headers.Set("te", "deflate")

	end2end = getEndToEndHeaders(headers)
	if !containsHeader(end2end, "content-type") {
		t.FailNow()
	}
	if containsHeader(end2end, "te") {
		t.FailNow()
	}

	headers = http.Header{}
	headers.Set("connection", "content-type")
	headers.Set("content-type", "text/csv")
	headers.Set("te", "deflate")
	end2end = getEndToEndHeaders(headers)
	if containsHeader(end2end, "connection") {
		t.FailNow()
	}
	if containsHeader(end2end, "content-type") {
		t.FailNow()
	}
	if containsHeader(end2end, "te") {
		t.FailNow()
	}

	headers = http.Header{}
	end2end = getEndToEndHeaders(headers)
	if len(end2end) != 0 {
		t.FailNow()
	}

	headers = http.Header{}
	headers.Set("connection", "content-type")
	end2end = getEndToEndHeaders(headers)
	if len(end2end) != 0 {
		t.FailNow()
	}
	tearDownTest()
}
