git2godir=Godeps/_workspace/src/github.com/libgit2/git2go
libgit2dir=$(git2godir)/vendor/libgit2

update-git2go:
	godep update github.com/libgit2/git2go/...
	$(MAKE) vendor-libgit2
	$(MAKE) build-libgit2

# vendorizes the libgit2 submodule in git2go (otherwise it won't be
# checked into this repo)
vendor-libgit2:
	rm -rf $(libgit2dir)/.git
	rm -rf $(libgit2dir)/build

build-libgit2:
	chmod +x $(git2godir)/script/*.sh
	GOPATH=$(shell godep path) $(MAKE) -C $(git2godir) install

install: build-libgit2
	godep go install -ldflags '-extldflags=-LGodeps/_workspace/src/github.com/libgit2/git2go/vendor/libgit2/build' ./cmd/vcsstore

test:
	go list ./... | grep -v cluster | xargs godep go test -tags lgtest -ldflags '-extldflags=-LGodeps/_workspace/src/github.com/libgit2/git2go/vendor/libgit2/build'
