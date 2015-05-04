package server

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"sort"

	"github.com/sourcegraph/mux"
	"golang.org/x/tools/godoc/vfs"
	"sourcegraph.com/sourcegraph/go-vcs/vcs"
	"sourcegraph.com/sourcegraph/vcsstore/fileutil"
	"sourcegraph.com/sourcegraph/vcsstore/vcsclient"
)

func (h *Handler) serveRepoTreeEntry(w http.ResponseWriter, r *http.Request) error {
	v := mux.Vars(r)

	repo, _, done, err := h.getRepo(r)
	if err != nil {
		return err
	}
	defer done()

	commitID, canon, err := getCommitID(r)
	if err != nil {
		return err
	}

	type fileSystem interface {
		FileSystem(vcs.CommitID) (vfs.FileSystem, error)
	}
	if repo, ok := repo.(fileSystem); ok {
		fs, err := repo.FileSystem(commitID)
		if err != nil {
			return err
		}

		path := v["Path"]
		fi, err := fs.Lstat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return &httpError{http.StatusNotFound, err}
			}
			return err
		}

		e := newTreeEntry(fi)
		var respVal interface{} = e // the value written to the resp body as JSON

		if fi.Mode().IsDir() {
			entries, err := fs.ReadDir(path)
			if err != nil {
				return err
			}

			e.Entries = make([]*vcsclient.TreeEntry, len(entries))
			for i, fi := range entries {
				e.Entries[i] = newTreeEntry(fi)
			}
			sort.Sort(vcsclient.TreeEntriesByTypeByName(e.Entries))
		} else if fi.Mode().IsRegular() {
			f, err := fs.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()

			contents, err := ioutil.ReadAll(f)
			if err != nil {
				return err
			}

			e.Contents = contents

			// Check for extended range options (GetFileOptions).
			var fopt vcsclient.GetFileOptions
			if err := schemaDecoder.Decode(&fopt, r.URL.Query()); err != nil {
				return err
			}
			if empty := (vcsclient.GetFileOptions{}); fopt != empty {
				fr, _, err := fileutil.ComputeFileRange(contents, fopt)
				if err != nil {
					return err
				}

				// Trim to only requested range.
				e.Contents = e.Contents[fr.StartByte:fr.EndByte]
				respVal = &vcsclient.FileWithRange{
					TreeEntry: e,
					FileRange: *fr,
				}
			}
		}

		if canon {
			setLongCache(w)
		} else {
			setShortCache(w)
		}
		return writeJSON(w, respVal)
	}

	return &httpError{http.StatusNotImplemented, fmt.Errorf("FileSystem not yet implemented for %T", repo)}
}

func newTreeEntry(fi os.FileInfo) *vcsclient.TreeEntry {
	e := &vcsclient.TreeEntry{
		Name:    fi.Name(),
		Size:    int(fi.Size()),
		ModTime: fi.ModTime(),
	}
	if fi.Mode().IsDir() {
		e.Type = vcsclient.DirEntry
	} else if fi.Mode().IsRegular() {
		e.Type = vcsclient.FileEntry
	} else if fi.Mode()&os.ModeSymlink != 0 {
		e.Type = vcsclient.SymlinkEntry
	}
	return e
}
