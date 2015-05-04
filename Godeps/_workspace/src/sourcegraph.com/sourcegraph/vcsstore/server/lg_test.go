// +build lgtest

package server

import (
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"sourcegraph.com/sourcegraph/go-vcs/vcs"
	_ "sourcegraph.com/sourcegraph/go-vcs/vcs/git"
	_ "sourcegraph.com/sourcegraph/go-vcs/vcs/hg"
	_ "sourcegraph.com/sourcegraph/go-vcs/vcs/hgcmd"
	"sourcegraph.com/sourcegraph/vcsstore"
	"sourcegraph.com/sourcegraph/vcsstore/vcsclient"
)

var (
	sshKeyFile  = flag.String("sshkey", "", "ssh private key file for clone remote")
	privateRepo = flag.String("privrepo", "ssh://git@github.com/sourcegraph/private-repo.git", "a private, SSH-accessible repo to test cloning")
)

func TestCloneGitHTTPS_lg(t *testing.T) {
	t.Parallel()
	testClone_lg(t, "git", "https://github.com/sgtest/empty-repo.git", vcs.RemoteOpts{}, "", 0)
}

func TestCloneGitGit_lg(t *testing.T) {
	t.Parallel()
	testClone_lg(t, "git", "git://github.com/sgtest/empty-repo.git", vcs.RemoteOpts{}, "", 0)
}

func TestCloneGitSSH_lg(t *testing.T) {
	t.Parallel()
	if *sshKeyFile == "" {
		t.Skip("no ssh key specified")
	}

	var opt vcs.RemoteOpts
	if *sshKeyFile != "" {
		key, err := ioutil.ReadFile(*sshKeyFile)
		if err != nil {
			log.Fatal(err)
		}
		opt.SSH = &vcs.SSHConfig{PrivateKey: key}
	}

	testClone_lg(t, "git", *privateRepo, opt, "", 0)
}

func TestCloneGitSSH_noKey_lg(t *testing.T) {
	t.Parallel()
	testClone_lg(t, "git", *privateRepo, vcs.RemoteOpts{}, "authentication required but no callback set", http.StatusUnauthorized)
}

func TestCloneGitSSH_emptyKey_lg(t *testing.T) {
	t.Parallel()
	opt := vcs.RemoteOpts{SSH: &vcs.SSHConfig{}}
	testClone_lg(t, "git", *privateRepo, opt, "callback returned unsupported credentials type", http.StatusUnauthorized)
}

func TestCloneGitSSH_badKey_lg(t *testing.T) {
	t.Parallel()
	opt := vcs.RemoteOpts{
		SSH: &vcs.SSHConfig{PrivateKey: []byte(badKey)},
	}
	testClone_lg(t, "git", *privateRepo, opt, "Failed to authenticate SSH session: Waiting for USERAUTH response", http.StatusForbidden)
}

func TestCloneHgHTTPS_lg(t *testing.T) {
	t.Parallel()
	testClone_lg(t, "hg", "https://bitbucket.org/sqs/go-vcs-hgtest", vcs.RemoteOpts{}, "", 0)
}

func testClone_lg(t *testing.T, vcsType, repoURLStr string, opt vcs.RemoteOpts, wantCloneErrStr string, wantCloneErrHTTPStatus int) {
	storageDir, err := ioutil.TempDir("", "vcsstore-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(storageDir)

	conf := &vcsstore.Config{
		StorageDir: storageDir,
		Log:        log.New(os.Stderr, "", 0),
		DebugLog:   log.New(os.Stderr, "", log.LstdFlags),
	}

	h := NewHandler(vcsstore.NewService(conf), nil, nil)
	h.Log = log.New(os.Stderr, "", 0)
	h.Debug = true

	srv := httptest.NewServer(h)
	defer srv.Close()

	baseURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	c := vcsclient.New(baseURL, nil)
	repoURL, err := url.Parse(repoURLStr)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := c.Repository(vcsType, repoURL)
	if err != nil {
		t.Fatal(err)
	}

	if repo, ok := repo.(vcsclient.RepositoryCloneUpdater); ok {
		// Clones the first time.
		err := repo.CloneOrUpdate(opt)
		checkErr(t, err, wantCloneErrStr, wantCloneErrHTTPStatus)

		// Updates the second time.
		err = repo.CloneOrUpdate(opt)
		checkErr(t, err, wantCloneErrStr, wantCloneErrHTTPStatus)
	} else {
		t.Fatalf("Remote cloning is not implemented for %T.", repo)
	}
}

func checkErr(t *testing.T, err error, wantErrStr string, wantCloneErrHTTPStatus int) {
	if wantErrStr == "" && err != nil {
		t.Fatal(err)
	}
	if wantErrStr != "" && (err == nil || !strings.Contains(err.Error(), wantErrStr)) {
		t.Fatalf("got error %q, want it to contain %q", err, wantErrStr)
	}
	if wantCloneErrHTTPStatus != 0 {
		if errResp, ok := err.(*vcsclient.ErrorResponse); ok && errResp.HTTPStatusCode() != wantCloneErrHTTPStatus {
			t.Fatalf("got error HTTP status code %d, want %d", errResp.HTTPStatusCode(), wantCloneErrHTTPStatus)
		}
	}
}

const badKey = `-----BEGIN RSA PRIVATE KEY-----
MIIEogIBAAKCAQEAvn3mKiRQE20MOoUfIfMxYjC5CO/1odFx21lhlUO2JRxcX6aj
XcUYbihJBDh2ynnNtdn+1yZc5pkhsm7AEdCcNMeoaPo/iQX3aVPVc+DbtZ70r41D
g8ZuGJheY6b1CsP5Fpg1tPHLH6XV6FLb236VYg21kkQNpqpBcCCY4uDO7uSlTveV
Sbe4J4XiLQfaaidfbbrEwWMnQPmnGRSZVz6DVgeM6EpGb9bWRKN39I0ZxtKWrjzb
ePVhKdkiShSM5gO6ui/PHrBcC/Eaq9EnmL9RH3yPMW4q8xcVPhJlpSWVQrzi90iy
24iFynsRlBp+5D1fvuK1B+QY5Hta9n2dhZQuxwIDAQABAoIBAHNA8VVmCxz1yYRO
rvt3jNV/7TQ/GzsA4gZ5GdWZ1ka11h35ULaXXaSosyTelOEEuTXh45KBH4xV8lFn
OMaOlTRJ6Nc7Md3RwlPz6R3nWoeU2S6uJb9A+7Pd3J4mkfQlhjvpc/U6pk9LoxSh
rWwsNm3RJZ6NlkTUlislTdtXcVyP5PObdRn5+Ki+igQHFleI7LKLpHBzAlzDiOou
PBD7J1eF4Xf0iGxs2To9yrM5RuEsoHXERlgDjBvV9I1BbveCfaEI+2bdEmLlJ1eJ
FBg4CrWEqzPW692IL/X46WJClnki7UHxmPSAh0KEHK1gCBZ261dHcYQ9WJXq9+pA
agKdfAECgYEA/BXEYNYK2qD+HDvvE8ZqS9ndQJr3soSZC3tCLrbvfv7btnFpna2V
nKmt4A9ZvAxHdl3sWIwUFdZ4BtYCrJ7ilhNI9Cn6XvXQVwC8hwc0zOthKLa/UcuZ
yf5r4iaLQzIcOR310OUxNisNo/E2/NHy95I5mX0ST351za51av2amUcCgYEAwXNA
UbpPzVDfMp8Lb2MNcP0mejuZe7XBBsCCjVbyH56ynD5i1sWuZqzct9V5O+8myWyP
28/d4nSnbOI5nms5E0ZnaF9w84F1U2DOXFTTjjakUW3IZAlTbClBO8JKGrl11Kkd
0e/6hkYhOZBrY1mMHnuC2ynUT+yLmI8S1eDkfoECgYANU/VLDWYLgyGMSprsV7w9
AGrTRJ4+AQa6dazdHWzyMPVa4worfQcA/nOj+gvLhnasynB5igZx1SIJcn03tTrT
pndf+Ww0Yxi90Nsm5HmlL/i2F1tsLrCV3m7DyTfpuJeHaY8amVONwp75AQLgQRVw
g3mqJNO4Aj6mPkgU/Q2UdwKBgAgeC+7iAIM/B36aSeKMp328QacTZSdZwxXDcjb4
FQTapegEfiVA+kZ4rnJQVNv89wWwtoCkwkzEVFovS/enzCdQ5vnsN1MgdYngIAij
zpTDGjYIg0YfVg7N1FzrlCx258jap9OtXDfSLYa61qa+lTCaQi1sHeqUpG7sYf/z
heMBAoGATxyqG8vavVB/0I7LYevRtze3Js8ntajcveVp9lB0zmM5341W1P9MvsGv
Eah6HEEShDAgD4tbQMatFB51Q5JKNDczMFII7tUdx4iI/U+N/hXxFtOvKdqydfuA
EMozjwkIRp7HZLjILETldc+QEZiiD44/1wuvJrPIoQSzHeXbwQk=
-----END RSA PRIVATE KEY-----`
