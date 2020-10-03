#!/bin/bash

set -e

RUN_PATH="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
cd $RUN_PATH

echo ----[ Create .deb ]----
DEB_FILE=services-execution_${VERSION}_amd64.deb
DEB_PATH=$RUN_PATH/build/debian_out/services-execution
rm -rf $DEB_PATH
mkdir -p $DEB_PATH $DEB_PATH/DEBIAN/ $DEB_PATH/usr/sbin/

cat > $DEB_PATH/DEBIAN/control << _EOF
Package: services-execution
Version: $VERSION
Maintainer: Foilen
Architecture: amd64
Description: This is an application that runs as root and that executes multiples applications.
_EOF

cp -rv DEBIAN $DEB_PATH/
cp -rv build/bin/* $DEB_PATH/usr/sbin/

cd $DEB_PATH/..
dpkg-deb --no-uniform-compression --build services-execution
mv services-execution.deb $DEB_FILE
