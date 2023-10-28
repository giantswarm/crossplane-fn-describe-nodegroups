#!/bin/bash
go generate ./...
docker build . -t docker.io/choclab/function-describe-nodegroups:v0.0.1
docker push choclab/function-describe-nodegroups:v0.0.1
