// Generation is disabled for now because we're not compatible with the latest version of vcsstore.
// Use the currently pinned older version for now.
//#go:generate go run gen.go

// Command vcsstorecmd is a fork of sourcegraph.com/sourcegraph/vcsstore/cmd/vcsstore,
// but it uses gitcmd backend instead of native git one.
package main
