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
gofmt -l \
  "$GW/gateway/ipacl.go" "$GW/gateway/ipacl_test.go" "$GW/gateway/handler.go" \
  "$GW/gateway/router.go" "$GW/gateway/config.go" "$GW/gateway/blockpage.go" \
  "$GW/gateway/reload.go" "$GW/gateway/reload_test.go" "$GW/gateway/engine.go" \
  "$GW/gateway/health.go" "$GW/gateway/events.go" "$GW/gateway/events_test.go" \
  "$GW/gateway/ratelimit.go" "$GW/gateway/ratelimit_test.go" "$GW/gateway/bans.go" \
  "$GW/gateway/attack.go" "$GW/gateway/attack_test.go" \
  "$GW/gateway/lists.go" "$GW/gateway/lists_test.go" \
  "$GW/gateway/admin.go" "$GW/gateway/admin_test.go" \
  "$GW/gateway/rulepolicy_test.go" \
  "$GW/main.go" || true

echo "=== coraza-gateway: go build ==="
( cd "$GW" && go build ./... )
echo "=== coraza-gateway: go vet ==="
( cd "$GW" && go vet ./... )
echo "=== coraza-gateway: go test ==="
( cd "$GW" && go test ./... )

echo "=== gofmt check (touched agent files) ==="
gofmt -l \
  "$AGENT/utils/wafconfig/ipacl.go" \
  "$AGENT/utils/wafconfig/ipacl_test.go" \
  "$AGENT/utils/wafconfig/config.go" \
  "$AGENT/utils/wafconfig/config_test.go" \
  "$AGENT/utils/wafconfig/ratelimit.go" \
  "$AGENT/utils/wafconfig/ratelimit_test.go" \
  "$AGENT/utils/wafconfig/lists.go" \
  "$AGENT/utils/wafconfig/lists_test.go" \
  "$AGENT/utils/wafconfig/rulepolicy.go" \
  "$AGENT/utils/wafconfig/rulepolicy_test.go" \
  "$AGENT/app/service/waf_list.go" \
  "$AGENT/app/service/waf_ban.go" \
  "$AGENT/app/model/waf.go" \
  "$AGENT/app/repo/waf.go" \
  "$AGENT/app/dto/request/waf.go" \
  "$AGENT/app/dto/response/waf.go" \
  "$AGENT/app/service/waf_control.go" \
  "$AGENT/app/service/waf_control_test.go" \
  "$AGENT/app/api/v2/waf.go" \
  "$AGENT/router/ro_website.go" \
  "$AGENT/init/hook/hook.go" || true

echo "=== agent/utils/wafconfig: go vet ==="
( cd "$AGENT" && go vet ./utils/wafconfig/... )
echo "=== agent/utils/wafconfig: go test ==="
( cd "$AGENT" && go test ./utils/wafconfig/... )

echo "=== agent: go vet (api/router/hook) ==="
( cd "$AGENT" && go vet ./app/api/v2/ ./router/ ./init/hook/ )

echo "=== agent/app/service: go vet (WAF control) ==="
( cd "$AGENT" && go vet ./app/service/ )
echo "=== agent/app/service: go test (WAF focused) ==="
( cd "$AGENT" && go test ./app/service/ -count=1 -v -run 'Waf|IPLines|PolicyMode|WebsiteOrigin|RootProxy|ApplyNginx|ApplyGatewayConfig|ComposeAsset|RateLimit|AttackLimit' 2>&1 | grep -E '^(--- |ok|FAIL|PASS)' )
