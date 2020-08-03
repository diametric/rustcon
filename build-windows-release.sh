#!/bin/bash

VERSION=$(cat VERSION)

GOOS=windows ./build.sh

mkdir -p builds/windows/rustcon-$VERSION
mkdir -p builds/windows/rustcon-$VERSION/scripts

cp rustcon.exe builds/windows/rustcon-$VERSION/
cp rustcon.conf builds/windows/rustcon-$VERSION/
cp scripts/* builds/windows/rustcon-$VERSION/scripts/

cd builds/windows
zip -r rustcon-$VERSION.zip rustcon-$VERSION/

cd ../../
rm -rf builds/windows/rustcon-$VERSION/

