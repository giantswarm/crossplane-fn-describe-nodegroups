#!/bin/bash
go build . && {
    rm package/*.xpkg
    go generate ./...
    docker buildx build . -t docker.io/choclab/function-describe-nodegroups:v0.0.1
    crossplane xpkg build -f package --embed-runtime-image=docker.io/choclab/function-describe-nodegroups:v0.0.1
    crossplane xpkg push -f package/$(ls package | grep function-describe) docker.io/choclab/function-describe-nodegroups:v0.0.1
}