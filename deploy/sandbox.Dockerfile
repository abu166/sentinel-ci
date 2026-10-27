FROM golang:1.23-alpine
RUN apk add --no-cache python3 py3-pytest build-base time ca-certificates && mkdir -p /workspace && chown 65532:65532 /workspace
ENV HOME=/tmp GOCACHE=/workspace/.gocache GOPATH=/workspace/.gopath GOMAXPROCS=2 GOFLAGS=-p=2 GOTOOLCHAIN=local GOPROXY=off PYTHONDONTWRITEBYTECODE=1
USER 65532:65532
WORKDIR /workspace
