#!/bin/bash

VERSION=v0.0.27
go build . && {
    rm package/*.xpkg
    go generate ./...
    docker buildx build . -t docker.io/choclab/function-describe-nodegroups:${VERSION}
    crossplane xpkg build -f package --embed-runtime-image=docker.io/choclab/function-describe-nodegroups:${VERSION}
    crossplane xpkg push -f package/$(ls package | grep function-describe) docker.io/choclab/function-describe-nodegroups:${VERSION}
}
