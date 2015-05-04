package datad

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"testing"

	"github.com/coreos/etcd/config"
	"github.com/coreos/etcd/etcd"
	etcd_client "github.com/coreos/go-etcd/etcd"
)

var etcdDebug = flag.Bool("test.etcd-debug", false, "log all etcd client requests and responses")

func init() {
	flag.Parse()
	if *etcdDebug {
		etcd_client.SetLogger(log.New(os.Stderr, "etcd_client: ", 0))
	}
}

var testNum int

func withEtcd(t *testing.T, f func(*etcd_client.Client)) {
	c := config.New()

	c.Name = fmt.Sprintf("TEST%d", testNum)
	c.Addr = fmt.Sprintf("127.0.0.1:%d", 4401+testNum)
	c.Peer.Addr = fmt.Sprintf("127.0.0.1:%d", 7701+testNum)
	testNum++

	tmpdir, err := ioutil.TempDir("", "datad-etcd-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	c.DataDir = tmpdir
	c.Force = true

	c.Peer.HeartbeatInterval = 25
	c.Peer.ElectionTimeout = 100
	c.SnapshotCount = 10000

	i := etcd.New(c)
	go i.Run()
	<-i.ReadyNotify()

	// Run f.
	f(etcd_client.NewClient([]string{i.Server.URL()}))

	i.Stop()
}

func TestEtcdBackend(t *testing.T) {
	withEtcd(t, func(ec *etcd_client.Client) {
		b := NewEtcdBackend("/p", ec)
		testBackend(t, b)
	})
}

func must(t *testing.T, err error) {
	if err != nil {
		t.Fatal(err)
	}
}
