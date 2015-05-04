Hgo is a collection of Go packages providing read-access to
local Mercurial repositories. Only a subset of Mercurial's
functionality is supported. It is possible to access revisions
of files and to read changelogs, manifests, and tags.

Hgo supports the following repository features:

	* revlogv1
	* store
	* fncache (no support for hash encoded names, though)
	* dotencode

The Go packages have been implemented from scratch, based
on information found in Mercurial's wiki.

The project should be considered unstable. The BUGS file lists
known issues yet to be addressed.

cmd/hgo contains an example program that implements a few
commands similar to a subset of Mercurial's hg.
