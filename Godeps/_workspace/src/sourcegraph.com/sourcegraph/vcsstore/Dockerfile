FROM ubuntu:14.04

RUN apt-get update -q
RUN apt-get install -qy build-essential curl git mercurial pkg-config

# Install Go
RUN curl -Ls https://golang.org/dl/go1.3.3.linux-amd64.tar.gz | tar -C /usr/local -xz
ENV PATH /usr/local/go/bin:$PATH
ENV GOBIN /usr/local/bin

RUN apt-get install -qy cmake libssh2-1-dev libssl-dev

# Install hglib (for hg blame)
RUN apt-get install -qy python-hglib

ENV GOPATH /opt
RUN go get github.com/tools/godep
ADD . /opt/src/sourcegraph.com/sourcegraph/vcsstore
WORKDIR /opt/src/sourcegraph.com/sourcegraph/vcsstore
RUN make build-libgit2
RUN make install

# Trust GitHub's SSH host key (for ssh cloning of GitHub repos)
RUN mkdir -m 700 -p /root/.ssh
RUN cp etc/github_known_hosts /root/.ssh/known_hosts
RUN chmod 600 /root/.ssh/known_hosts

EXPOSE 80
VOLUME ["/mnt/vcsstore"]
CMD ["-v", "-s=/mnt/vcsstore", "serve", "-http=:80"]
ENTRYPOINT ["/usr/local/bin/vcsstore"]
