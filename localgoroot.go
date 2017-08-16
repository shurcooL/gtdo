package main

import (
	"go/build"
	"io/ioutil"
	"path/filepath"
)

var LocalGoVersion string

func localGoVersion() (string, error) {
	version, err := ioutil.ReadFile(filepath.Join(build.Default.GOROOT, "VERSION"))
	if err != nil {
		return "", err
	}
	return string(version), nil
}
