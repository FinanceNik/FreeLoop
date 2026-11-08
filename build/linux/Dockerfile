# The Linux build and test machine (NFR-XPLAT-003), run by `make linux-*` through
# Docker. Ubuntu 22.04 for the oldest common glibc, so the binary runs on it and
# anything newer; WebKitGTK 4.1 (Wails' gtk3 build tag), which Ubuntu 22.04+,
# Debian 12+ and Fedora ship. Go and Node are pinned to what the Mac build uses.
FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl git build-essential pkg-config file xz-utils \
      libgtk-3-dev libwebkit2gtk-4.1-dev \
      bubblewrap dbus dbus-x11 gnome-keyring libsecret-tools openssh-client \
      xvfb xauth \
    && rm -rf /var/lib/apt/lists/*

# Checked against the SHA-256 the projects publish (go.dev/dl, nodejs.org SHASUMS256.txt).
ARG GO_VERSION=1.27.3
ARG GO_SHA256_amd64=65efbc80fdc1fa2cd2631cad5b481777a68e8725f34e89f50fc6ac714e379b40
ARG GO_SHA256_arm64=a06e99309dbcbdca1a8ccd2282c3ee146e55930886e26b4f1193269587165f49
ARG NODE_VERSION=22.22.1
ARG NODE_SHA256_amd64=9a6bc82f9b491279147219f6a18add1e18424dce90d41d2a5fcd69d4924ba3aa
ARG NODE_SHA256_arm64=0f3550d58d45e5d3cf7103d9e3f69937f09fe82fb5dd474c66a5d816fa58c9ee
RUN arch=$(dpkg --print-architecture); nodearch=$([ "$arch" = arm64 ] && echo arm64 || echo x64) \
 && go_sha=$([ "$arch" = arm64 ] && echo "$GO_SHA256_arm64" || echo "$GO_SHA256_amd64") \
 && node_sha=$([ "$arch" = arm64 ] && echo "$NODE_SHA256_arm64" || echo "$NODE_SHA256_amd64") \
 && curl -fsSLo /tmp/go.tgz "https://go.dev/dl/go${GO_VERSION}.linux-${arch}.tar.gz" \
 && echo "${go_sha}  /tmp/go.tgz" | sha256sum -c - \
 && tar -C /usr/local -xzf /tmp/go.tgz \
 && curl -fsSLo /tmp/node.txz "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${nodearch}.tar.xz" \
 && echo "${node_sha}  /tmp/node.txz" | sha256sum -c - \
 && tar -C /usr/local --strip-components=1 -xJf /tmp/node.txz \
 && rm /tmp/go.tgz /tmp/node.txz
ENV PATH=/usr/local/go/bin:/root/go/bin:$PATH GOFLAGS=-buildvcs=false

# A user to run as: tests that check a file is unreadable mean nothing as root.
RUN useradd -m -u 1000 build && mkdir -p /home/build/go /home/build/.cache && chown -R build:build /home/build
USER build
ENV HOME=/home/build GOPATH=/home/build/go GOCACHE=/home/build/.cache/go-build
# Tests commit in scratch repositories, as a developer's machine would allow.
RUN git config --global user.name build && git config --global user.email build@localhost
WORKDIR /src
