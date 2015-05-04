package vcsstore

import (
	"net/url"
	"path/filepath"
	"strings"
)

// EncodeRepositoryPath encodes the VCS type and clone URL of a repository into
// a path suitable for use in a URL. The encoded path may be decoded with
// DecodeRepositoryPath, which is roughly the inverse operation (except for
// calling filepath.Clean on the URL path).
func EncodeRepositoryPath(vcsType string, cloneURL *url.URL) string {
	hostPart := cloneURL.Host
	if cloneURL.User != nil {
		hostPart = cloneURL.User.Username() + "@" + hostPart
	}
	return strings.Join([]string{vcsType, cloneURL.Scheme, hostPart, strings.TrimPrefix(filepath.Clean(cloneURL.Path), "/")}, "/")
}

// DecodeRepositoryPath decodes a repository path encoded using RepositoryPath.
func DecodeRepositoryPath(path string) (vcsType string, cloneURL *url.URL, err error) {
	parts := strings.SplitN(path, "/", 4)
	if len(parts) != 4 {
		tmp := make([]string, 4)
		copy(tmp, parts)
		parts = tmp
	}
	vcsType = parts[0]
	host, userinfo := parseHostAndUserinfo(parts[2])
	cloneURL = &url.URL{Scheme: parts[1], Host: host, User: userinfo, Path: parts[3]}
	return vcsType, cloneURL, nil
}

func parseHostAndUserinfo(s string) (string, *url.Userinfo) {
	delim := strings.Index(s, "@")
	if delim <= 0 || delim == len(s) {
		return s, nil
	}

	userRaw := s[:delim]
	host := s[delim+1:]
	return host, url.User(userRaw)
}
