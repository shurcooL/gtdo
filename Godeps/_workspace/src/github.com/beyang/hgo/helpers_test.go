package hgo_test

import (
	"flag"
	"io/ioutil"
	"os"
	"os/exec"
	"testing"
	"time"
)

var (
	keepTmpDirs = flag.Bool("test.keeptmp", false,
		"don't remove temporary dirs after use")

	// tmpDirs is used by makeTmpDir and removeTmpDirs to record and clean up
	// temporary directories used during testing.
	tmpDirs []string
)

// Convert time to OS X compatible `touch -t` time
func appleTime(t string) string {
	ti, _ := time.Parse(time.RFC3339, t)
	return ti.Local().Format("200601021504.05")
}

func createRepo(t testing.TB, commands []string) string {
	dir := makeTmpDir(t)

	for _, command := range commands {
		cmd := exec.Command("bash", "-c", command)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Command %q failed. Output was:\n\n%s", cmd, out)
		}
	}

	return dir
}

// removeTmpDirs removes all temporary directories created by makeTmpDir
// (unless the -test.keeptmp flag is true, in which case they are retained).
func removeTmpDirs(t testing.TB) {
	if *keepTmpDirs {
		return
	}
	for _, dir := range tmpDirs {
		err := os.RemoveAll(dir)
		if err != nil {
			t.Fatalf("tearDown: RemoveAll(%q) failed: %s", dir, err)
		}
	}
	tmpDirs = nil
}

// makeTmpDir creates a temporary directory and returns its path. The
// directory is added to the list of directories to be removed when the
// currently running test ends (assuming the test calls removeTmpDirs() after
// execution).
func makeTmpDir(t testing.TB) string {
	dir, err := ioutil.TempDir("", "hgo-")
	if err != nil {
		t.Fatal(err)
	}

	if *keepTmpDirs {
		t.Logf("Using temp dir %s.", dir)
	}

	tmpDirs = append(tmpDirs, dir)
	return dir
}
