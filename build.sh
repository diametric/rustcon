#!/bin/bash

VERSION=$(cat ./VERSION)
GIT_REVISION=$(git rev-parse --short HEAD)
TIME=$(date)

go build -ldflags="-X 'github.com/diametric/rustcon/version.BuildVersion=$VERSION' -X 'github.com/diametric/rustcon/version.BuildTime=$TIME' -X 'github.com/diametric/rustcon/version.GitRevision=$GIT_REVISION'" .


