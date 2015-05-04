package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"testing"

	"sourcegraph.com/sourcegraph/go-vcs/vcs"
)

func TestServeRepo(t *testing.T) {
	setupHandlerTest()
	defer teardownHandlerTest()

	cloneURL, _ := url.Parse("git://a.b/c")
	sm := &mockServiceForExistingRepo{
		t:        t,
		vcs:      "git",
		cloneURL: cloneURL,
	}
	testHandler.Service = sm

	resp, err := http.Get(server.URL + testHandler.router.URLToRepo("git", cloneURL).String())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if !sm.opened {
		t.Errorf("!opened")
	}
	if got, want := resp.StatusCode, http.StatusOK; got != want {
		t.Errorf("got code %d, want %d", got, want)
		logResponseBody(t, resp)
	}
}

func TestServeRepo_DoesNotExist(t *testing.T) {
	setupHandlerTest()
	defer teardownHandlerTest()

	cloneURL, _ := url.Parse("git://a.b/c")
	var calledOpen bool
	sm := &mockService{
		t:        t,
		vcs:      "git",
		cloneURL: cloneURL,
		open: func(vcs string, cloneURL *url.URL) (interface{}, error) {
			// Simulate that the repository doesn't exist locally.
			calledOpen = true
			return nil, os.ErrNotExist
		},
		clone: func(vcs string, cloneURL *url.URL, opt vcs.RemoteOpts) (interface{}, error) {
			t.Fatal("unexpectedly called Clone")
			panic("unreachable")
		},
	}
	testHandler.Service = sm

	req, err := http.NewRequest("GET", server.URL+testHandler.router.URLToRepo("git", cloneURL).String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if !calledOpen {
		t.Errorf("!calledOpen")
	}
	if got, want := resp.StatusCode, http.StatusNotFound; got != want {
		t.Errorf("got code %d, want %d", got, want)
		logResponseBody(t, resp)
	}
}

func TestServeRepoCreateOrUpdate_CreateNew_noBody(t *testing.T) {
	setupHandlerTest()
	defer teardownHandlerTest()

	cloneURL, _ := url.Parse("git://a.b/c")
	rm := struct{}{} // trivial mock repository
	var calledOpen, calledClone bool
	sm := &mockService{
		t:        t,
		vcs:      "git",
		cloneURL: cloneURL,
		open: func(vcs string, cloneURL *url.URL) (interface{}, error) {
			// Simulate that the repository doesn't exist locally.
			calledOpen = true
			return nil, os.ErrNotExist
		},
		clone: func(vcs string, cloneURL *url.URL, opt vcs.RemoteOpts) (interface{}, error) {
			calledClone = true
			return rm, nil
		},
	}
	testHandler.Service = sm

	req, err := http.NewRequest("POST", server.URL+testHandler.router.URLToRepo("git", cloneURL).String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if !calledOpen {
		t.Errorf("!calledOpen")
	}
	if !calledClone {
		t.Errorf("!calledClone")
	}
	if got, want := resp.StatusCode, http.StatusCreated; got != want {
		t.Errorf("got code %d, want %d", got, want)
		logResponseBody(t, resp)
	}
}

func TestServeRepoCreateOrUpdate_CreateNew_withBody(t *testing.T) {
	setupHandlerTest()
	defer teardownHandlerTest()

	cloneURL, _ := url.Parse("git://a.b/c")
	opt := vcs.RemoteOpts{SSH: &vcs.SSHConfig{User: "u"}}
	rm := struct{}{} // trivial mock repository
	var calledOpen, calledClone bool
	sm := &mockService{
		t:        t,
		vcs:      "git",
		cloneURL: cloneURL,
		opt:      opt,
		open: func(vcs string, cloneURL *url.URL) (interface{}, error) {
			// Simulate that the repository doesn't exist locally.
			calledOpen = true
			return nil, os.ErrNotExist
		},
		clone: func(vcs string, cloneURL *url.URL, opt vcs.RemoteOpts) (interface{}, error) {
			calledClone = true
			return rm, nil
		},
	}
	testHandler.Service = sm

	body, _ := json.Marshal(opt)
	req, err := http.NewRequest("POST", server.URL+testHandler.router.URLToRepo("git", cloneURL).String(), bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if !calledOpen {
		t.Errorf("!calledOpen")
	}
	if !calledClone {
		t.Errorf("!calledClone")
	}
	if got, want := resp.StatusCode, http.StatusCreated; got != want {
		t.Errorf("got code %d, want %d", got, want)
		logResponseBody(t, resp)
	}
}

func TestServeRepoCreateOrUpdate_UpdateExisting_noBody(t *testing.T) {
	setupHandlerTest()
	defer teardownHandlerTest()

	cloneURL, _ := url.Parse("git://a.b/c")
	rm := &mockUpdateEverythinger{t: t}
	sm := &mockServiceForExistingRepo{
		t:        t,
		vcs:      "git",
		cloneURL: cloneURL,
		repo:     rm,
	}
	testHandler.Service = sm

	req, err := http.NewRequest("POST", server.URL+testHandler.router.URLToRepo("git", cloneURL).String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if !sm.opened {
		t.Errorf("!opened")
	}
	if !rm.called {
		t.Errorf("!called")
	}
	if got, want := resp.StatusCode, http.StatusOK; got != want {
		t.Errorf("got code %d, want %d", got, want)
		logResponseBody(t, resp)
	}
}

func TestServeRepoCreateOrUpdate_UpdateExisting_withBody(t *testing.T) {
	setupHandlerTest()
	defer teardownHandlerTest()

	cloneURL, _ := url.Parse("git://a.b/c")
	opt := vcs.RemoteOpts{SSH: &vcs.SSHConfig{User: "u"}}
	rm := &mockUpdateEverythinger{t: t, opt: opt}
	sm := &mockServiceForExistingRepo{
		t:        t,
		vcs:      "git",
		cloneURL: cloneURL,
		repo:     rm,
	}
	testHandler.Service = sm

	body, _ := json.Marshal(opt)
	req, err := http.NewRequest("POST", server.URL+testHandler.router.URLToRepo("git", cloneURL).String(), bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if !sm.opened {
		t.Errorf("!opened")
	}
	if !rm.called {
		t.Errorf("!called")
	}
	if got, want := resp.StatusCode, http.StatusOK; got != want {
		t.Errorf("got code %d, want %d", got, want)
		logResponseBody(t, resp)
	}
}

type mockUpdateEverythinger struct {
	t *testing.T

	// expected args
	opt vcs.RemoteOpts

	// return values
	err error

	called bool
}

func (m *mockUpdateEverythinger) UpdateEverything(opt vcs.RemoteOpts) error {
	m.called = true
	if !reflect.DeepEqual(opt, m.opt) {
		m.t.Errorf("mock: got opt %+v, want %+v", asJSON(opt), asJSON(m.opt))
	}
	return m.err
}

type mockServiceForExistingRepo struct {
	t *testing.T

	// expected args
	vcs      string
	cloneURL *url.URL

	// return values
	repo interface{}
	err  error

	opened bool
}

func (m *mockServiceForExistingRepo) Open(vcs string, cloneURL *url.URL) (interface{}, error) {
	if vcs != m.vcs {
		m.t.Errorf("mock: got vcs arg %q, want %q", vcs, m.vcs)
	}
	if cloneURL.String() != m.cloneURL.String() {
		m.t.Errorf("mock: got cloneURL arg %q, want %q", cloneURL, m.cloneURL)
	}
	m.opened = true
	return m.repo, m.err
}

func (m *mockServiceForExistingRepo) Clone(vcs string, cloneURL *url.URL, opt vcs.RemoteOpts) (interface{}, error) {
	m.t.Errorf("mock: unexpectedly called Clone for repo that exists (%s %s)", vcs, cloneURL)
	return m.repo, m.err
}

func (m *mockServiceForExistingRepo) Close(vcs string, cloneURL *url.URL) {}

type mockService struct {
	t *testing.T

	// expected args
	vcs      string
	cloneURL *url.URL
	opt      vcs.RemoteOpts

	// mockable methods
	open  func(vcs string, cloneURL *url.URL) (interface{}, error)
	clone func(vcs string, cloneURL *url.URL, opt vcs.RemoteOpts) (interface{}, error)
}

func (m *mockService) Open(vcs string, cloneURL *url.URL) (interface{}, error) {
	if m.vcs != "" && vcs != m.vcs {
		m.t.Errorf("mock: got vcs arg %q, want %q", vcs, m.vcs)
	}
	if m.cloneURL != nil && cloneURL.String() != m.cloneURL.String() {
		m.t.Errorf("mock: got cloneURL arg %q, want %q", cloneURL, m.cloneURL)
	}
	return m.open(vcs, cloneURL)
}

func (m *mockService) Clone(vcs string, cloneURL *url.URL, opt vcs.RemoteOpts) (interface{}, error) {
	if vcs != m.vcs {
		m.t.Errorf("mock: got vcs arg %q, want %q", vcs, m.vcs)
	}
	if cloneURL.String() != m.cloneURL.String() {
		m.t.Errorf("mock: got cloneURL arg %q, want %q", cloneURL, m.cloneURL)
	}
	if !reflect.DeepEqual(opt, m.opt) {
		m.t.Errorf("mock: got opt %+v, want %+v", asJSON(opt), asJSON(m.opt))
	}
	return m.clone(vcs, cloneURL, opt)
}

func (m *mockService) Close(vcs string, cloneURL *url.URL) {}

func asJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
