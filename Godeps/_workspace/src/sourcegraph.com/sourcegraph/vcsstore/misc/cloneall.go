package main

import (
	"errors"
	"flag"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var (
	cloneURLs = make(chan string)
	done      = make(chan res)

	par = flag.Int("p", runtime.GOMAXPROCS(0), "parallelism")
)

func main() {
	flag.Parse()

	log.SetFlags(0)

	urlBytes, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		log.Fatal(err)
	}

	urls := strings.Split(string(urlBytes), "\n")
	log.Printf("%d clone URLs", len(urls))

	go pump(urls)

	for i := 0; i < *par; i++ {
		go cloner()
	}

	start := time.Now()
	var errs, skips, clones int
	for res := range done {
		_, err := res.url, res.err
		if err == nil {
			clones++
		} else if err == errSkip {
			skips++
			continue
		} else if err != nil {
			errs++
			continue
		}

		pct := float64(clones) / float64(len(urls)) * 100
		freq := float64(clones) / float64(time.Since(start)/time.Second)
		rem := time.Duration(float64(len(urls)-(clones)) / freq * float64(time.Second))
		log.Printf("% 6d/% 6d cloned (%.1f%%, %.3f/sec, %s remaining) [%d errs, %d skips]", clones, len(urls)-errs-skips, pct, freq, rem, errs, skips)
	}

	select {}
}

func pump(urls []string) {
	for _, url := range urls {
		cloneURLs <- url
	}
}

func roundDuration(d time.Duration) time.Duration {
	return (d / 1e6) * 1e6
}

func cloner() {
	for {
		urlStr := <-cloneURLs
		start := time.Now()
		urlStr = strings.Replace(urlStr, "https://", "git://", -1)
		u, err := url.Parse(urlStr)
		if err != nil {
			log.Printf("%s: url parse failed: %s", urlStr, err)
			done <- res{urlStr, err}
			continue
		}
		dir := dir(u)
		if err := os.MkdirAll(filepath.Dir(dir), 0700); err != nil {
			log.Printf("%s: mkdir failed: %s", urlStr, err)
			done <- res{urlStr, err}
			continue
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			// skip; exists already
			done <- res{urlStr, errSkip}
			continue
		}
		c := exec.Command("git", "clone", "--mirror", "--bare", urlStr, dir)
		if err := cmdRunWithTimeout(2*time.Minute, c); err != nil {
			log.Printf("%s: clone failed: %s", urlStr, err)
			done <- res{urlStr, err}
			continue
		}
		log.Printf("%10s %s", roundDuration(time.Since(start)), urlStr)
		done <- res{urlStr, nil}
	}
}

var errSkip = errors.New("skip")

type res struct {
	url string
	err error
}

func dir(u *url.URL) string {
	return filepath.Join("/mnt/vcsstore/git/https", u.Host, strings.TrimSuffix(u.Path, ".git")+".git")
}

var errCmdTimeout = errors.New("command timed out")

func cmdRunWithTimeout(timeout time.Duration, cmd *exec.Cmd) error {
	errc := make(chan error, 1)
	go func() {
		errc <- cmd.Run()
	}()
	var err error
	select {
	case <-time.After(timeout):
		cmd.Process.Kill()
		return errCmdTimeout
	case err = <-errc:
	}
	return err
}
