package main

import (
	"fmt"
	"log"
	"net/http"
	"sync"
)

type importPathBranch struct {
	importPath string
	branch     string
}

var (
	sseMu sync.Mutex
	sse   map[importPathBranch][]pageViewer
)

type pageViewer struct {
	id       *http.ResponseWriter
	outdated chan struct{}
}

// NotifyOutdated is called by repo updater when the given page viewer is outdated.
// It returns immediately.
func (pv *pageViewer) NotifyOutdated() {
	select {
	case pv.outdated <- struct{}{}:
	default:
	}
}

func eventsHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method != "GET" {
		w.Header().Set("Allow", "GET")
		log.Println("unexpected eventsHandler method:", req.Method)
		http.Error(w, "Expected method to be GET... Does EventSource need something else too?", http.StatusMethodNotAllowed)
		return
	}

	query := req.URL.Query()

	importPath := query.Get("ImportPath")
	importPathBranch := importPathBranch{
		importPath: importPath,
		branch:     query.Get("Branch"),
	}
	importPathRepoSpec := importPathRepoSpec{
		importPath: importPath,
		repoSpec: repoSpec{
			vcsType:  query.Get("RepoSpec.VCSType"),
			cloneURL: query.Get("RepoSpec.CloneURL"),
		},
	}
	if importPathRepoSpec.importPath == "" || importPathRepoSpec.vcsType == "" || importPathRepoSpec.cloneURL == "" {
		log.Println("Invalid importPathRepoSpec:", importPathRepoSpec)
		http.Error(w, "Invalid importPathRepoSpec.", http.StatusBadRequest)
		return
	}
	RepoUpdater.Enqueue(importPathRepoSpec)

	flusher, ok := w.(http.Flusher)
	if !ok {
		log.Println("Streaming unsupported.")
		http.Error(w, "Streaming unsupported.", http.StatusInternalServerError)
		return
	}

	closeNotifier, ok := w.(http.CloseNotifier)
	if !ok {
		log.Println("CloseNotifier unsupported.")
		http.Error(w, "CloseNotifier unsupported.", http.StatusInternalServerError)
		return
	}
	closeChan := closeNotifier.CloseNotify()

	outdatedChan := make(chan struct{}, 1)
	{
		log.Println("Client connection joined:", &w)
		sseMu.Lock()
		sse[importPathBranch] = append(sse[importPathBranch], pageViewer{
			id:       &w,
			outdated: outdatedChan,
		})
		sseMu.Unlock()
	}
	defer func() {
		sseMu.Lock()
		for i, pv := range sse[importPathBranch] {
			if pv.id == &w {
				// Delete without preserving order.
				sse[importPathBranch][i] = sse[importPathBranch][len(sse[importPathBranch])-1]
				sse[importPathBranch][len(sse[importPathBranch])-1] = pageViewer{}
				sse[importPathBranch] = sse[importPathBranch][:len(sse[importPathBranch])-1]
				if len(sse[importPathBranch]) == 0 {
					delete(sse, importPathBranch)
				}
				log.Println("Client connection gone away:", &w)
				break
			}
		}
		sseMu.Unlock()
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	/*w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")*/
	/*if *productionFlag {
		w.Header().Set("Access-Control-Allow-Origin", "https://gotools.org")
	}*/

	for {
		select {
		case <-outdatedChan:
			_, err := fmt.Fprintf(w, "data: %s\n\n", "outdated")
			if err != nil {
				log.Println("(via write error:", err)
				return
			}

			flusher.Flush()
		case <-closeChan:
			log.Println("(via CloseNotifier)")
			return
		}
	}
}
