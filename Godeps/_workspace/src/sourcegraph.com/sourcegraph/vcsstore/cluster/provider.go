package cluster

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"

	"sourcegraph.com/sourcegraph/datad"
	"sourcegraph.com/sourcegraph/go-vcs/vcs"
	"sourcegraph.com/sourcegraph/vcsstore"
)

type VCSDataProvider struct {
	conf *vcsstore.Config
	svc  vcsstore.Service
	Log  *log.Logger
}

func NewProvider(conf *vcsstore.Config, svc vcsstore.Service) datad.Provider {
	return &VCSDataProvider{
		conf: conf,
		svc:  svc,
		Log:  log.New(os.Stderr, "", log.Ltime|log.Lshortfile),
	}
}

func (p *VCSDataProvider) HasKey(key string) (bool, error) {
	vcsType, cloneURL, err := vcsstore.DecodeRepositoryPath(strings.TrimPrefix(key, "/"))
	if err != nil {
		return false, err
	}

	_, err = p.svc.Open(vcsType, cloneURL)
	if err != nil {
		return false, datad.ErrKeyNotExist
	}

	return true, nil
}

func (p *VCSDataProvider) Keys(keyPrefix string) ([]string, error) {
	keyPrefix = strings.TrimPrefix(keyPrefix, "/")
	keyPrefix = filepath.Clean(keyPrefix)
	if strings.HasPrefix(keyPrefix, "..") || strings.HasPrefix(keyPrefix, "/") {
		return nil, errors.New("invalid keyPrefix")
	}
	topDir := filepath.Join(p.conf.StorageDir, keyPrefix)

	var keys []string
	err := filepath.Walk(topDir, func(path string, info os.FileInfo, err error) error {
		// Ignore errors for broken symlinks.
		if err != nil {
			if info == nil {
				return err
			}
			if info.Mode()&os.ModeSymlink == 0 {
				return err
			}
		}

		if info.Mode().IsDir() {
			vcsTypes := []string{"git", "hg"}
			for _, vcsType := range vcsTypes {
				_, err := vcs.Open(vcsType, path)
				if err == nil {
					key, err := filepath.Rel(p.conf.StorageDir, path)
					if err != nil {
						return err
					}

					keys = append(keys, key)
					return filepath.SkipDir
				}
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return keys, nil
}

func (p *VCSDataProvider) Update(key string) error {
	key = strings.TrimPrefix(key, "/")
	vcsType, cloneURL, err := vcsstore.DecodeRepositoryPath(key)
	if err != nil {
		return err
	}

	cloned := false
	repo, err := p.svc.Open(vcsType, cloneURL)
	if os.IsNotExist(err) {
		cloned = true
		// TODO(sqs): add support for setting RemoteOpts (requires
		// persistence?).
		repo, err = p.svc.Clone(vcsType, cloneURL, vcs.RemoteOpts{})
	}
	if err != nil {
		return err
	}

	if !cloned {
		// No need to update it now if it was just cloned.
		err := updateRepository(repo)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *VCSDataProvider) logf(format string, a ...interface{}) {
	if p.Log != nil {
		p.Log.Printf(format, a...)
	}
}

func updateRepository(repo interface{}) error {
	type mirrorUpdate interface {
		MirrorUpdate() error
	}
	if repo, ok := repo.(mirrorUpdate); ok {
		return repo.MirrorUpdate()
	}
	return errors.New("failed to update repo")
}
