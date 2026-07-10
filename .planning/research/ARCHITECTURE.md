# 架构研究

## 系统边界

```text
Browser / API client
        |
        v
core (control plane)
  auth, session, global settings, logs, frontend assets
        |
        | local Unix socket or future authenticated node transport
        v
agent (execution plane)
  Docker, websites, databases, files, host, cron, ClamAV, AI agents
```

## 商业覆盖层

- Go 社区入口使用 `//go:build !xpack && !enterprise`。
- 私有构建通过同名 Router、Provider 和初始化文件覆盖社区空实现。
- Vue 使用 `import.meta.glob` 探测 `frontend/src/xpack` 与 `frontend/src/enterprise`。
- 这些目录和构建入口被 `.gitignore` 排除，公开仓库只保留接口、占位和部分通用 UI。

## 首版集成方式

### 主题与水印

```text
public login -> GET public enhancement subset -> apply base CSS variables
authenticated panel -> GET full enhancement settings -> render watermark
settings UI -> validate -> POST enhancement update -> core settings table
```

不修改许可证状态，不注册虚假的 xpack 路由。开放实现作为现有 extension fallback，未来存在私有扩展时仍可覆盖。

### Webhook 告警

```text
alert rule -> AlertSender -> community AlertProvider
           -> platform payload -> HTTPS webhook
           -> platform response validation -> AlertLog -> AlertTask attempt
```

失败投递也必须写日志并计为一次尝试，避免监控周期内无限重试。

### ClamAV 调度

```text
Clam rule DB row -> validate cron -> register local cron entry
cron callback -> existing scan task -> record parser -> existing alert pipeline
agent restart -> reload enabled rules -> replace in-memory entry IDs
```

调度所有权应位于 service/独立执行内核，不能从 xpack helper 反向导入 service 造成循环依赖。

### AI Agent 配额

```text
Create request -> name/port checks -> load AIAgentLimit
               -> 0: unlimited / >0: operator soft limit
               -> existing app install and agent lifecycle
```

## 后续构建顺序

1. 建立开放扩展注册和安全配置模式。
2. 完成单节点增强能力和测试门禁。
3. 设计节点身份、密钥轮换、mTLS/HMAC 和幂等任务协议。
4. 在节点协议稳定后实现资源同步和 RBAC。
5. 再实现 WAF/监控/防篡改等高数据量与安全敏感模块。
6. 最后处理数据库高可用、AI 网关和虚拟机。
