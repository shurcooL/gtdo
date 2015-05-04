package datad

// A Provider makes data accessible to the datad cluster.
type Provider interface {
	// HasKey returns whether this provider has the underlying data for key. If
	// not, it returns the error ErrKeyNotExist.
	HasKey(key string) (bool, error)

	// Keys returns a list of keys under keyPrefix.
	Keys(keyPrefix string) ([]string, error)

	// Update performs a synchronous update of this key's data from the
	// underlying data source. If key does not exist in this provider, it will
	// be created.
	Update(key string) error
}
