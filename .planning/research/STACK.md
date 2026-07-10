# 技术栈研究

**研究日期：** 2026-07-10  
**基线：** `upstream/dev-v2@8be2a9ab0270139d0cea2f023ea3f287db2217e0`

## 现有栈

| 层 | 技术 | 版本/约束 | 结论 |
|---|---|---|---|
| 控制面 | Go, Gin, GORM, SQLite | `core/go.mod` 要求 Go 1.26.1 | 保持现有 router -> api -> service -> repo -> model 分层 |
| 执行面 | Go, Docker API, Cron, systemd/主机工具 | `agent/go.mod` 标注 Go 1.25.10，已由 Go 1.26.1 编译验证 | 新的主机能力优先落在 agent，控制面只做协调和代理 |
| 前端 | Vue 3, Pinia, Element Plus, Vite | Node `^20.19 || >=22.12`，当前 Node 24.14 | 复用现有组件和动态扩展入口，不引入第二套 UI 框架 |
| 构建 | npm + Go 原生二进制 | 前端先写入 `core/cmd/server/web`，再由 `go:embed` 编入 core | 固定顺序：`npm ci` -> `build:pro` -> 编译 core/agent |
| 部署 | Linux 原生二进制 + systemd | 深度依赖 Docker、网络、防火墙、磁盘和 `/opt/1panel` | 构建可容器化，运行时不应伪装成普通隔离容器 |

## 首版新增实现

- 主题与水印：复用 core `settings` 键值表，提供公开安全子集和登录后完整设置 API。
- Webhook 告警：使用 Go 标准库 HTTP 客户端实现企业微信、钉钉、飞书公开机器人协议。
- ClamAV 定时扫描：复用 `robfig/cron/v3`、现有扫描任务、记录和告警模型。
- AI Agent 配额：使用 agent 设置表中的 `AIAgentLimit` 作为操作员软限制，`0` 表示不限数量。

## 构建建议

```bash
cd frontend
npm ci
npm run build:pro

cd ../core
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o ../build/1panel-core ./cmd/server/main.go

cd ../agent
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o ../build/1panel-agent ./cmd/server/main.go
```

## 不采用

- 不全局设置 `isProductPro=true`，否则会暴露没有后端的 WAF、多节点、集群等页面。
- 不复刻许可证协议，不生成或伪造许可证状态。
- 不把完整面板包装成需要 `--privileged`、host PID/network 和 Docker socket 的运行时容器；这种方式几乎没有隔离价值。
- 不在首版引入新的数据库、消息队列或前端状态框架。

## 置信度

- 现有构建链：高，已实际完成前端生产构建、Go 全包编译检查和 Linux AMD64 二进制构建。
- 首版四项能力与现有架构匹配度：高，公开代码已有 UI、模型或接口边界。
- 全部商业功能一次性实现：低，WAF、多节点、RBAC、数据库高可用和虚拟机分别属于独立安全域。
