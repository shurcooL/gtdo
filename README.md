# gtdo [![Build Status](https://travis-ci.org/shurcooL/gtdo.svg?branch=master)](https://travis-ci.org/shurcooL/gtdo) [![GoDoc](https://godoc.org/github.com/shurcooL/gtdo?status.svg)](https://godoc.org/github.com/shurcooL/gtdo)

gtdo is the source for [gotools.org](http://gotools.org/).

![Screenshot](Screenshot.png)

Installation
------------

At this time, `gtdo` needs to be built with [`godep`](https://github.com/tools/godep) tool as it relies on older pinned versions of some dependencies.

```bash
go get -u -d github.com/shurcooL/gtdo
cd $GOPATH/src/github.com/shurcooL/gtdo
godep go build -o /tmp/gtdo
```

Development
-----------

This package relies on `go generate` directives to process and statically embed assets. For development only, you'll need extra dependencies:

```bash
go get -u -d -tags=generate github.com/shurcooL/gtdo/...
go get -u -d -tags=js github.com/shurcooL/gtdo/...
```

Afterwards, you can build and run the package in development mode, where all assets are always read and processed from disk:

```bash
godep go build -tags=dev github.com/shurcooL/gtdo
```

When you're done with development, you should run `go generate` before committing:

```bash
go generate github.com/shurcooL/gtdo/...
```

License
-------

-	[MIT License](http://opensource.org/licenses/mit-license.php)
