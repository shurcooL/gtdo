package vcsclient

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"os"

	"golang.org/x/tools/godoc/vfs"
	"sourcegraph.com/sourcegraph/go-vcs/vcs"
)

type FileSystem interface {
	vfs.FileSystem
	Get(path string) (*TreeEntry, error)
}

type repositoryFS struct {
	at   vcs.CommitID
	repo *repository
}

var _ FileSystem = &repositoryFS{}

func (fs *repositoryFS) Open(name string) (vfs.ReadSeekCloser, error) {
	e, err := fs.Get(name)
	if err != nil {
		return nil, err
	}

	return nopCloser{bytes.NewReader(e.Contents)}, nil
}

func (fs *repositoryFS) Lstat(path string) (os.FileInfo, error) {
	e, err := fs.Get(path)
	if err != nil {
		return nil, err
	}

	return e.Stat()
}

func (fs *repositoryFS) Stat(path string) (os.FileInfo, error) {
	// TODO(sqs): follow symlinks (as Stat specification requires)
	e, err := fs.Get(path)
	if err != nil {
		return nil, err
	}

	return e.Stat()
}

func (fs *repositoryFS) ReadDir(path string) ([]os.FileInfo, error) {
	e, err := fs.Get(path)
	if err != nil {
		return nil, err
	}

	fis := make([]os.FileInfo, len(e.Entries))
	for i, e := range e.Entries {
		fis[i], err = e.Stat()
		if err != nil {
			return nil, err
		}
	}

	return fis, nil
}

func (fs *repositoryFS) String() string {
	return fmt.Sprintf("%s repository %s commit %s (client)", fs.repo.vcsType, fs.repo.cloneURL, fs.at)
}

// Get returns the whole TreeEntry struct for a tree entry.
func (fs *repositoryFS) Get(path string) (*TreeEntry, error) {
	url, err := fs.url(path, nil)
	if err != nil {
		return nil, err
	}

	req, err := fs.repo.client.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, err
	}

	var entry *TreeEntry
	_, err = fs.repo.client.Do(req, &entry)
	if err != nil {
		return nil, err
	}

	return entry, nil
}

// FileRange is a line and byte range in a file.
type FileRange struct {
	StartLine, EndLine int `url:",omitempty"` // start and end line range
	StartByte, EndByte int `url:",omitempty"` // start and end byte range
}

// FileWithRange is returned by GetFileWithOptions and includes the
// returned file's TreeEntry as well as the actual range of lines and
// bytes returned (based on the GetFileOptions parameters). That is,
// if Start/EndLine are set in GetFileOptions, this struct's
// Start/EndByte will be set to the actual start and end bytes of
// those specified lines, and so on for the other fields in
// GetFileOptions.
type FileWithRange struct {
	*TreeEntry
	FileRange // range of actual returned tree entry contents within file
}

// GetFileOptions specifies options for GetFileWithOptions.
type GetFileOptions struct {
	FileRange // line or byte range to fetch (can't set both line *and* byte range)

	// EntireFile is whether the entire file contents should be
	// returned. If true, Start/EndLine and Start/EndBytes are
	// ignored.
	EntireFile bool `url:",omitempty"`

	// ExpandContextLines is how many lines of additional output
	// context to include (if Start/EndLine and Start/EndBytes are
	// specified). For example, specifying 2 will expand the range to
	// include 2 full lines before the beginning and 2 full lines
	// after the end of the range specified by Start/EndLine and
	// Start/EndBytes.
	ExpandContextLines int `url:",omitempty"`

	// FullLines is whether a range that includes partial lines should
	// be extended to the nearest line boundaries on both sides. It is
	// only valid if StartByte and EndByte are specified.
	FullLines bool `url:",omitempty"`
}

// GetFileWithOptions gets a file and allows additional configuration
// of the range to return, etc.
func (fs *repositoryFS) GetFileWithOptions(path string, opt GetFileOptions) (*FileWithRange, error) {
	url, err := fs.url(path, opt)
	if err != nil {
		return nil, err
	}

	req, err := fs.repo.client.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, err
	}

	var file *FileWithRange
	_, err = fs.repo.client.Do(req, &file)
	if err != nil {
		return nil, err
	}

	return file, nil
}

// A FileGetter is a repository FileSystem that can get files with
// extended range options (GetFileWithOptions).
type FileGetter interface {
	GetFileWithOptions(path string, opt GetFileOptions) (*FileWithRange, error)
}

// url generates the URL to RouteRepoTreeEntry for the given path (all other
// route vars are taken from repositoryFS fields).
func (fs *repositoryFS) url(path string, opt interface{}) (*url.URL, error) {
	return fs.repo.url(RouteRepoTreeEntry, map[string]string{
		"CommitID": string(fs.at),
		"Path":     path,
	}, opt)
}

type nopCloser struct {
	io.ReadSeeker
}

func (nc nopCloser) Close() error { return nil }
