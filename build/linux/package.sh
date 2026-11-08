#!/bin/sh
# Runs inside the image from build/linux/Dockerfile (`make linux-package`): builds
# the shipped Linux app for the container's architecture and packages it as an
# AppImage and a .deb. /src is the repository, read-only, with frontend/dist
# already built on the host; /out is the host's bin/, where the results go:
#   /out/linux-<arch>/freeloop, THIRD_PARTY_NOTICES.txt, BUILDINFO.txt
#   /out/Freeloop-<version>-linux-<arch>.AppImage and .deb
set -eu
# linuxdeploy walks PATH and stops at a folder it cannot read: the image's
# /root/go/bin, as the build user. Nothing here is installed there.
export PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin
arch=$(go env GOARCH)
mkdir /tmp/app
cd /src
tar cf - --exclude=node_modules --exclude=.git --exclude=bin --exclude=.task . | tar xf - -C /tmp/app
cd /tmp/app
# The notices read license files from the frontend's packages; the host's are
# fine for that (only their native build tools differ by platform).
ln -s /src/frontend/node_modules frontend/node_modules
wails3 task linux:build:binary ARCH="$arch" SHIPPED=1
wails3 task linux:package:files ARCH="$arch"
{
	echo "linux/$arch built in build/linux/Dockerfile"
	. /etc/os-release && echo "$PRETTY_NAME, glibc $(ldd --version | head -1 | sed 's/.* //')"
	go version
	gcc --version | head -1
	echo "GTK $(pkg-config --modversion gtk+-3.0), WebKitGTK $(pkg-config --modversion webkit2gtk-4.1)"
	echo "linuxdeploy $(sha256sum /opt/linuxdeploy/linuxdeploy.AppImage | cut -d' ' -f1)"
	echo "wails3 $(wails3 version 2>&1 | tail -1)"
} > "bin/linux-$arch/BUILDINFO.txt"
mkdir -p "/out/linux-$arch"
cp "bin/linux-$arch/freeloop" "bin/linux-$arch/THIRD_PARTY_NOTICES.txt" "bin/linux-$arch/BUILDINFO.txt" "/out/linux-$arch/"
cp bin/Freeloop-*-linux-"$arch".AppImage bin/Freeloop-*-linux-"$arch".deb /out/
ls -l /out/Freeloop-*-linux-"$arch".*
