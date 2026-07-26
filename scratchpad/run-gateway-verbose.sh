#!/bin/bash
set -e
export GOROOT=/home/lainy/codex-go1.26.1/go
export PATH=$GOROOT/bin:/usr/bin:/bin
export GOFLAGS=-mod=mod
export GOCACHE=/home/lainy/.cache/go-build
export GOPATH=/home/lainy/go
cd /mnt/d/_CodeNotSync/_1Panel-X/source/coraza-gateway
go test ./gateway/ -count=1 -v 2>&1 | grep -E '^(--- |ok |FAIL|PASS)'
