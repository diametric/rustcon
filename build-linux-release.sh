#!/bin/bash

VERSION=$(cat VERSION)

GOOS=linux ./build.sh

mkdir -p builds/linux/rustcon-$VERSION
mkdir -p builds/linux/rustcon-$VERSION/scripts

cp rustcon builds/linux/rustcon-$VERSION/
cp rustcon.conf builds/linux/rustcon-$VERSION/
cp scripts/* builds/linux/rustcon-$VERSION/scripts/

cd builds/linux
tar -zcvf rustcon-$VERSION.tar.gz rustcon-$VERSION/

cd ../../
rm -rf builds/linux/rustcon-$VERSION/

