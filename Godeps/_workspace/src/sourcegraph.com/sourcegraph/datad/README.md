# datad

[![Build Status](https://travis-ci.org/sourcegraph/datad.svg)](https://travis-ci.org/sourcegraph/datad)

A distributed cache that spreads an existing local data source across a cluster,
routes requests for data to the appropriate nodes, and ensures data is
replicated and available.

datad was created, and is (almost ready to be) used in production, at
[Sourcegraph](https://sourcegraph.com) to provide fast, reliable access to 4TB+
of git and hg repository data (files, commits, branches, etc.).

**WARNING:** This is a very new project. Use at your own risk!

## Architecture

* **Data source:** any existing local data source, keyed on some function of your choice. E.g., git repository data (keyed on clone URL).
* **Provider:** an interface to the data source on the local machine with methods for ensuring a copy of the data exists on disk, updating the data, and enumerating all of the keys of data.
* **Registry:** two mappings: (1) for a given data key, a list of cluster nodes that have the underlying data on disk; and (2) for a given node, a list of data keys that it should fetch/compute and store on disk.
* **Node:** a member of the cluster that hosts a subset of the data from its local data source, which it continuously synchronizes with the registry.
* **Client:** a consumer of the data source that routes its requests for data to the nodes that are registered for any given data key.

## Tests

Run `go test`.

There are also good tests in [sourcegraph.com/sourcegraph/vcsstore](https://sourcegraph.com/sourcegraph/vcsstore) in the `cluster` package.

## TODO

* Support keeping a list of data keys that must always be available.
* Allow nodes to indicate that they are in the process of fetching a piece of data for the first time. As it stands currently, if it takes (e.g.) 5s for the first fetch, and a client requests the key 2x before fetching finishes, then the first request will register the key to the node and the second request will deregister the key from the node (because the key transport notices the node failed to respond successfully). Because registration is done using (pseudo-) "consistent hashing", it usually gets registered to the same node on the next request, and by that time the node has the key, but this still causes unnecessary traffic and errors in the meantime.
* Allow nodes to indicate they don't want more keys to be registered to them (e.g., when their disk is full).
* Make the Provider.Keys method return keys as it finds them on disk, instead of waiting until it's found all of them.
* When the provider is registering existing keys on disk, the watcher catches them and dupes an update. Just make the watcher not watch existing-registered keys.
* Rebalances continue to reassign to dead nodes.