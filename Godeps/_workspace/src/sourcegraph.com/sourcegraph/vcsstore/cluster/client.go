package cluster

import (
	"errors"
	"net/http"
	"net/url"

	"sourcegraph.com/sourcegraph/datad"
	"sourcegraph.com/sourcegraph/go-vcs/vcs"
	"sourcegraph.com/sourcegraph/vcsstore"
	"sourcegraph.com/sourcegraph/vcsstore/vcsclient"
)

// A Client accesses repositories distributed across a datad cluster.
type Client struct {
	// datad is the underlying datad client to use.
	datad *datad.Client

	// transport is the underlying HTTP transport to use.
	transport http.RoundTripper
}

// NewClient creates a new client to access repositories distributed in a datad
// cluster.
func NewClient(dc *datad.Client, t http.RoundTripper) *Client {
	return &Client{dc, t}
}

var _ vcsclient.RepositoryOpener = &Client{}

func (c *Client) TransportForRepository(vcsType string, cloneURL *url.URL) (*datad.KeyTransport, error) {
	key := vcsstore.EncodeRepositoryPath(vcsType, cloneURL)
	return c.datad.TransportForKey(key, c.transport)
}

// Repository implements vcsclient.RepositoryOpener.
func (c *Client) Repository(vcsType string, cloneURL *url.URL) (vcs.Repository, error) {
	repo, err := c.Open(vcsType, cloneURL)
	if err != nil {
		return nil, err
	}

	if repo, ok := repo.(vcs.Repository); ok {
		return repo, nil
	}

	return nil, errors.New("repository does not support this operation")
}

// Open implements vcsstore.Service and opens a repository. If the repository
// does not exist in the cluster, an os.ErrNotExist-satisfying error is
// returned.
func (c *Client) Open(vcsType string, cloneURL *url.URL) (interface{}, error) {
	key := vcsstore.EncodeRepositoryPath(vcsType, cloneURL)

	t, err := c.TransportForRepository(vcsType, cloneURL)
	if err != nil {
		return nil, err
	}

	vc := vcsclient.New(nil, &http.Client{Transport: t})
	repo, err := vc.Repository(vcsType, cloneURL)
	if err != nil {
		return nil, err
	}
	return &repository{c.datad, key, repo, t}, nil
}

// Clone implements vcsstore.Service and clones a repository.
func (c *Client) Clone(vcsType string, cloneURL *url.URL, opt vcs.RemoteOpts) (interface{}, error) {
	key := vcsstore.EncodeRepositoryPath(vcsType, cloneURL)

	_, err := c.datad.Update(key)
	if err != nil {
		return nil, err
	}

	// TODO(sqs): add option for waiting for clone (triggered by Update) to
	// complete?

	return c.Open(vcsType, cloneURL)
}

func (c *Client) Close(vcsType string, cloneURL *url.URL) {
	// TODO(sqs): can this be used to make the cluster faster?
}

var (
	_ vcsclient.RepositoryOpener = &Client{}
	_ vcsstore.Service           = &Client{}
)

// repository wraps a vcsclient.repository to make CloneOrUpdate also add the
// repository key to the datad registry.
type repository struct {
	datad    *datad.Client
	datadKey string
	vcs.Repository
	keyTransport *datad.KeyTransport
}

func (r *repository) CloneOrUpdate(opt vcs.RemoteOpts) error {
	_, err := r.datad.Update(r.datadKey)
	if err != nil {
		return nil
	}

	// Update the node list in this transport. The transport is used by all of
	// the other standard vcs.Repository methods on *repository. Updating the
	// node list here lets the other methods be called on *this* *repository
	// instead of having to get a new KeyTransport from the datad.Client.
	err = r.keyTransport.SyncWithRegistry()
	if err != nil {
		return err
	}

	// TODO(sqs): doing double work here? Update triggers a clone, and we call CloneOrUpdate.

	if rrc, ok := r.Repository.(vcsclient.RepositoryCloneUpdater); ok {
		return rrc.CloneOrUpdate(opt)
	}

	return nil
}
