#!/bin/bash
set -e
export GOROOT=/home/lainy/codex-go1.26.1/go
export PATH=$GOROOT/bin:/usr/bin:/bin
export GOFLAGS=-mod=mod
export GOCACHE=/home/lainy/.cache/go-build
export GOPATH=/home/lainy/go

GW=/mnt/d/_CodeNotSync/_1Panel-X/source/coraza-gateway
AGENT=/mnt/d/_CodeNotSync/_1Panel-X/source/agent

echo "=== gofmt check (touched gateway files) ==="
gofmt -l "$GW/gateway/ipacl.go" "$GW/gateway/ipacl_test.go" "$GW/gateway/handler.go" "$GW/gateway/router.go" "$GW/gateway/config.go" "$GW/gateway/blockpage.go" || true

echo "=== coraza-gateway: go vet ==="
( cd "$GW" && go vet ./... )
echo "=== coraza-gateway: go test ==="
( cd "$GW" && go test ./... )

echo "=== gofmt check (touched agent files) ==="
gofmt -l \
  "$AGENT/utils/wafconfig/ipacl.go" \
  "$AGENT/utils/wafconfig/ipacl_test.go" \
  "$AGENT/utils/wafconfig/config.go" \
  "$AGENT/app/model/waf.go" \
  "$AGENT/app/repo/waf.go" \
  "$AGENT/app/dto/request/waf.go" \
  "$AGENT/app/dto/response/waf.go" \
  "$AGENT/app/service/waf_control.go" \
  "$AGENT/app/service/waf_control_test.go" || true

echo "=== agent/utils/wafconfig: go vet ==="
( cd "$AGENT" && go vet ./utils/wafconfig/... )
echo "=== agent/utils/wafconfig: go test ==="
( cd "$AGENT" && go test ./utils/wafconfig/... )

echo "=== agent/app/service: go vet (WAF control) ==="
( cd "$AGENT" && go vet ./app/service/ )
echo "=== agent/app/service: go test (WAF focused) ==="
( cd "$AGENT" && go test ./app/service/ -run 'Waf|SplitIPLines|NormalizedPolicyMode|NormalizedWebsiteOrigin|SwitchRootProxy|ApplyNginx' )
