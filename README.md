gtdo
====

[![Go Reference](https://pkg.go.dev/badge/github.com/shurcooL/gtdo.svg)](https://pkg.go.dev/github.com/shurcooL/gtdo)

gtdo is the source for [gotools.org](https://gotools.org/).

![Screenshot](Screenshot.png)

Installation
------------

```sh
go install github.com/shurcooL/gtdo@latest
```

Development
-----------

This package relies on `go generate` directives to process and statically embed assets. For development only, you may need extra dependencies. You can build and run the package in development mode, where all assets are always read and processed from disk:

```sh
go build -tags=dev github.com/shurcooL/gtdo
```

When you're done with development, you should run `go generate` before committing:

```sh
go generate github.com/shurcooL/gtdo/...
```

License
-------

-	[MIT License](LICENSE)
