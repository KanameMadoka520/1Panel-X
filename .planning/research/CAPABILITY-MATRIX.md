# 1Panel-X 商业能力公开清单（功能矩阵）

> **来源**：2026-07-10 对 `open-pro-v1` @ `508403749` 真实源码的逐项探针核验（7 探针簇 + 3 公开资料研究 + 综合 + 对抗式评审，12 agent / 0 错误 / 53 能力）。每行 OSS 状态与门控类型均有 `file:line` 佐证（见 `.planning/research/` 探针原始结论与 workflow journal）。
> **诚实边界**：`gatingType=none_exists_full` = 社区版已完整开放（非缺口）。`uiGateOnly` 集合为**空**——不存在"仅前端/许可证门控、后端已完整"的免费翻牌能力；每个真实商业能力都需新建后端/数据模型。

## 关键结论
- **已完全开放（4 项，非缺口）**：网站备份增强、网站恢复策略、API 接口 IP 白名单、进阶主题（v1 之上）。
- **适合近期里程碑 `fitNextMilestone=true`（4 项）**：安全评分、自定义品牌/白标、自定义登录页、进阶主题——除安全评分外均属"开放增强设置服务"同一接缝（= v1.2 品牌簇候选）。
- **keystone 多节点注册**：15+ 能力依赖它（全部节点功能、RBAC、自定义应用仓库、增强代理、Skills Hub、全部数据库 HA）——最解锁但最高风险，需独立威胁模型里程碑，且需第二台真实 VPS 才能 UAT。
- **五大独立安全域**（多节点 / 高级 WAF / 防篡改 / 数据库 HA / 虚拟机）各需专门里程碑，不得捆绑。

## uiGateOnly（仅 UI/许可证门控、后端已完整）
- （空）——无此类能力。

## needNewDataModel（需从零新建数据模型+后端）
- Multi-node registration / enrollment (nodes/simple_nodes tables + enrollment handler + agent-install orchestration)
- Users management (users table + CRUD + session role plumbing)
- Roles (roles + permission-tree tables)
- Operations reports (report model + section aggregators)
- Scheduled report export (export-history model + cron registration)
- Security scoring / assessment (scoring model over existing subsystem reads)
- Website access monitoring PV/UV/IP (time-series model + nginx log tailer/parser; shared by QPS, request-log, ranking rows)
- WAF custom rules (rule/ACL model + engine management plane)
- WAF ACL / IP groups (IP-list + IP-group model)
- WAF region / geo rules (geo model + GeoIP DB pipeline)
- WAF attack logs and records (per-site log store reader + model)
- WAF trends/statistics/attack map (aggregation model over log store)
- Website anti-tamper protection (tampers table + file-immutability enforcement)
- Anti-tamper protect/exclude rules (rule/template model)
- Anti-tamper audit log and recovery (audit-log + recovery model)
- Skills Hub producer (skill_hub table + import/scan/review/version workflow)
- AI benchmark testing (benchmark task + result-metrics model + harness runner)
- AI gateway (ai_proxy_* table family + proxy dataplane + token accounting)
- Model download manager (local-model registry over shared model dir)
- vLLM management (vLLM CRUD model/router, mirror TensorRTLLM)
- MySQL high availability (cluster/replication topology model)
- PostgreSQL high availability (cluster topology model)
- Redis high availability (sentinel/cluster topology model)
- KVM / libvirt virtual machines (vm_isos/vm_networks/vm_storages/vm_snapshots/vm templates + enterprise route group)
- Local AI site building — Upage (whole builder+deploy data model)

## 风险由低到高排序
1. Advanced theme beyond v1
2. API allowlist (IP white list + key/timestamp)
3. Website backup enhancement
4. Website restore / recovery strategy
5. Advanced theme / branding: Custom branding / white-label
6. Enhanced custom login page
7. View vs manage permission granularity
8. Website request logs analysis
9. Website traffic / QPS monitoring
10. Website access monitoring PV/UV/IP
11. Website source/device/URI ranking
12. Security scoring / assessment
13. Node grouping
14. WAF custom block page
15. Model download manager
16. vLLM management
17. Users management (beyond single admin)
18. Roles
19. Node health & version
20. Node resource overview
21. Enhanced proxy management (Docker proxy sync)
22. Custom application repository
23. Scheduled report export
24. Operations reports
25. AI benchmark testing
26. Node credential / identity rotation
27. Independent SMS delivery
28. WAF attack logs and records
29. WAF region / geo rules
30. WAF trends/statistics/attack map
31. WAF rule/config export-import
32. WAF ACL — IP/UA/URL allow-deny + IP groups
33. WAF custom rules
34. Anti-tamper protect/exclude rules
35. Website anti-tamper protection
36. Anti-tamper audit log and recovery
37. Skills Hub
38. Node upgrade & rollback
39. Node-scoped permissions / RBAC
40. Node authentication & Core↔Agent mutual auth
41. Multi-node registration / enrollment
42. Cross-node sync of files/images/SSL/apps/config
43. Redis high availability
44. AI gateway (routing / content-audit / token-metering)
45. MySQL high availability / failover
46. PostgreSQL high availability
47. Local AI site building — Upage
48. PWA / mobile client
49. VM templates
50. VM VNC console
51. VM networking and storage
52. VM snapshots
53. KVM / libvirt virtual machines

## 功能矩阵（53 项）

| # | 能力 | OSS结构 | 门控类型 | 归属 | 数据模型 | 网络/权限风险 | 文件系统/破坏风险 | 自动测试 | 成本 | 优先级 | 适合下一里程碑 | 外部依赖 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 1 | Multi-node registration / enrollment (节点注册/添加) | partial | xpack_provider | mixed | high | critical | high | low | XL | P0 | — | SSH/sshd on target host; remote 1panel-agent binary install; sudo/root on target |
| 2 | Node authentication & Core↔Agent mutual auth (节点认证/双向认证) | partial | no_op_helper | mixed | medium | critical | low | low | L | P0 | — | TLS/x509 cert generation; reverse-proxy trust core<->agent |
| 3 | Node credential / identity rotation (节点凭据/证书轮换) | partial | xpack_provider | mixed | medium | high | medium | low | M | P2 | — | x509 re-issuance; SSH to push new credentials |
| 4 | Node grouping (节点分组) | present | no_op_helper | core | low | low | low | medium | S | P1 | — | - |
| 5 | Node health & version (节点健康/版本) | partial | xpack_provider | mixed | medium | medium | low | medium | M | P1 | — | - |
| 6 | Node upgrade & rollback (节点升级/回滚) | partial | xpack_provider | mixed | high | high | high | low | L | P2 | — | agent release/package artifacts; SSH/file push of packages; systemd restart on node |
| 7 | Node resource overview (多机概览) | partial | xpack_provider | mixed | medium | medium | low | medium | M | P1 | — | - |
| 8 | Cross-node sync of files/images/SSL/apps/config (跨节点同步) | partial | no_op_helper | mixed | very_high | high | high | low | XL | P2 | — | cross-host file transfer channel; image registry/prefix; Docker restart for proxy sync |
| 9 | WAF custom rules (自定义规则/ACL rule engine) | partial | missing_backend_logic | mixed | high | high | medium | low | XL | P2 | — | OpenResty/1pwaf Lua WAF engine; nginx reload |
| 10 | WAF ACL — IP/UA/URL allow-deny + IP groups (黑白名单) | partial | missing_backend_logic | mixed | medium | high | medium | low | L | P2 | — | OpenResty/1pwaf engine; remote IP-list source |
| 11 | WAF region / geo rules (地区访问限制) | none | missing_data_model | mixed | medium | high | low | low | L | P3 | — | GeoIP/MaxMind DB; OpenResty/1pwaf engine |
| 12 | WAF attack logs and records (拦截日志/封锁记录) | none | missing_data_model | mixed | high | medium | low | medium | L | P2 | — | 1pwaf engine log stream; per-site log storage (1pwaf/data/db) |
| 13 | WAF trends/statistics/attack map (拦截地图/统计看板) | none | entirely_absent | mixed | medium | low | low | medium | L | P3 | — | WAF log/records store; GeoIP for map; frontend map/chart component |
| 14 | WAF rule/config export-import (规则/IP组导入导出) | none | missing_backend_logic | mixed | medium | medium | medium | medium | M | P3 | — | WAF rule/IP-group model (also absent) |
| 15 | WAF custom block page (自定义拦截页面) | none | missing_backend_logic | mixed | low | medium | low | medium | S | P2 | — | OpenResty/1pwaf engine (serves html) |
| 16 | Website anti-tamper protection (网站防篡改) | partial | no_op_helper | mixed | medium | low | high | low | L | P2 | — | Linux file-immutability / fanotify / chattr / kernel file lock; runs on node host fs |
| 17 | Anti-tamper protect/exclude rules (保护/排除目录规则) | none | missing_data_model | mixed | medium | low | high | low | M | P3 | — | file-lock backend |
| 18 | Anti-tamper audit log and recovery (防篡改日志与恢复) | none | missing_data_model | mixed | high | low | high | low | M | P3 | — | fanotify/inotify audit stream; file backup/restore store |
| 19 | Website access monitoring PV/UV/IP (浏览量/访客数/独立IP) | partial | xpack_provider | mixed | high | medium | low | medium | L | P2 | — | nginx/OpenResty access.log; GeoIP DB (访客地图); User-Agent parser |
| 20 | Website traffic / QPS monitoring (实时流量/QPS) | partial | xpack_provider | mixed | high | low | low | medium | M | P2 | — | nginx/OpenResty access.log |
| 21 | Website request logs analysis (请求日志分析) | partial | xpack_provider | mixed | medium | low | low | high | M | P2 | — | nginx/OpenResty access.log (structured log_format) |
| 22 | Website source/device/URI ranking (访问统计排行) | partial | xpack_provider | mixed | high | medium | low | medium | M | P2 | — | User-Agent parser (os/browser/device); Referer classification (spider list); GeoIP DB |
| 23 | Website backup enhancement (网站备份增强) | present | none_exists_full | agent | medium | medium | medium | high | XS | P3 | — | tar/gzip; cloud storage SDKs (S3/OSS) |
| 24 | Website restore / recovery strategy (网站恢复/回滚策略) | present | none_exists_full | agent | medium | medium | high | medium | XS | P3 | — | tar/gzip; docker exec nginx -s reload; database recover sub-handlers |
| 25 | Users management (beyond single admin) 用户管理 | partial | missing_data_model | mixed | high | high | low | medium | L | P2 | — | - |
| 26 | Roles 角色 | partial | missing_data_model | mixed | high | high | low | medium | L | P2 | — | - |
| 27 | Node-scoped permissions / RBAC 节点级权限 | partial | no_op_helper | mixed | high | critical | low | low | L | P3 | — | - |
| 28 | View vs manage permission granularity 查看/管理权限粒度 | partial | missing_backend_logic | mixed | medium | high | low | medium | M | P3 | — | - |
| 29 | API allowlist (IP white list + key/timestamp) API 接口 IP 白名单 | present | none_exists_full | core | low | medium | low | high | XS | P3 | — | - |
| 30 | Operations reports 运维报表 | partial | missing_backend_logic | mixed | high | medium | medium | medium | XL | P1 | — | report/export renderer (PDF/HTML/xlsx) |
| 31 | Scheduled report export 定时导出报表 | partial | missing_backend_logic | mixed | medium | low | medium | medium | M | P2 | — | cron scheduler (present) + file writer to OpsReportSavePath |
| 32 | Security scoring / assessment 安全评分/评估 | partial | missing_backend_logic | mixed | high | low | low | medium | L | P1 | ✅ | - |
| 33 | Custom application repository (自定义应用仓库) | partial | xpack_provider | mixed | medium | medium | medium | medium | M | P1 | — | tar.gz extraction; remote repo host |
| 34 | Enhanced proxy management (Docker proxy sync) | partial | xpack_provider | mixed | low | medium | medium | medium | S | P1 | — | Docker daemon (daemon.json / systemd drop-in) |
| 35 | Skills Hub (import/scan/review/version/node install) | partial | xpack_provider | mixed | high | high | high | low | L | P2 | — | GitHub; 7z/unzip/tar; agent containers (openclaw/hermes) |
| 36 | AI benchmark testing (基准测试) | none | missing_data_model | mixed | medium | medium | low | medium | L | P2 | — | benchmark/load tool (genai-perf/vllm bench); HF tokenizers; GPU model endpoint |
| 37 | AI gateway (routing / content-audit / token-metering) | partial | xpack_provider | mixed | very_high | critical | medium | low | XL | P3 | — | Elasticsearch; embedding model (llama.cpp + Qwen3-Embedding); upstream LLM providers |
| 38 | Model download manager (模型下载器) | partial | xpack_provider | mixed | medium | high | medium | medium | M | P1 | — | Hugging Face Hub; ModelScope; hf/hf-transfer downloader |
| 39 | vLLM management | partial | missing_backend_logic | mixed | medium | medium | medium | low | L | P1 | — | Docker; NVIDIA/Intel/Ascend GPU runtime; vLLM images |
| 40 | MySQL high availability / failover (主从/故障转移) | partial | xpack_provider | agent | high | high | high | none | XL | P3 | — | mysqld replication/binlog; orchestrator/MHA failover; ProxySQL/HAProxy VIP |
| 41 | PostgreSQL high availability (高可用) | partial | xpack_provider | agent | high | high | high | none | XL | P3 | — | Patroni or repmgr; etcd/consul; PgBouncer/HAProxy |
| 42 | Redis high availability (高可用) | partial | xpack_provider | agent | high | high | medium | low | L | P3 | — | Redis Sentinel or Cluster; HAProxy/VIP |
| 43 | KVM / libvirt virtual machines (KVM 虚拟机) | partial | xpack_provider | mixed | very_high | high | critical | none | XL | P3 | — | libvirt/libvirtd; qemu-kvm; /dev/kvm (VT-x/AMD-V); virsh |
| 44 | VM networking and storage (虚拟机网络与存储) | partial | xpack_provider | agent | high | critical | high | none | L | P3 | — | libvirt virtual networks; Linux bridge (br0); libvirt storage pools |
| 45 | VM VNC console (虚拟机 VNC 控制台) | partial | xpack_provider | mixed | low | high | low | none | L | P3 | — | libvirt/qemu VNC graphics; websockify / noVNC proxy |
| 46 | VM snapshots (虚拟机快照) | partial | xpack_provider | agent | medium | low | critical | none | M | P3 | — | libvirt snapshot API / qemu-img; qcow2 backing files |
| 47 | VM templates (虚拟机模板) | partial | xpack_provider | agent | medium | low | high | none | M | P3 | — | libvirt domain XML define/dumpxml; qemu-img convert; virt-sysprep |
| 48 | PWA / mobile client (移动客户端/PWA) | none | entirely_absent | frontend | low | low | low | medium | L | P3 | — | - |
| 49 | Local AI site building — Upage (AI 建站) | partial | missing_backend_logic | mixed | very_high | medium | medium | low | XL | P3 | — | AI model/LLM provider; site template/render engine |
| 50 | Independent SMS delivery (独立短信告警) | present | no_op_helper | agent | medium | high | low | medium | L | P3 | — | third-party SMS gateway (Aliyun/Tencent Cloud); SMS credits/quota account |
| 51 | Custom branding / white-label (自定义 Logo/网站图标/面板品牌) | partial | xpack_provider | mixed | low | low | medium | high | M | P2 | ✅ | - |
| 52 | Enhanced custom login page (自定义登录页/欢迎语/背景) | partial | xpack_provider | mixed | low | low | medium | high | M | P2 | ✅ | - |
| 53 | Advanced theme beyond v1 (进阶主题：预设色板/登录按钮色) | present | none_exists_full | frontend | low | low | low | high | S | P2 | ✅ | - |

## 依赖图（关键边）
```
Node authentication & Core↔Agent mutual auth <- Multi-node registration / enrollment
Node credential / identity rotation <- Node authentication & Core↔Agent mutual auth, Multi-node registration / enrollment
Node grouping <- Multi-node registration / enrollment
Node health & version <- Multi-node registration / enrollment, Node authentication & Core↔Agent mutual auth
Node upgrade & rollback <- Multi-node registration / enrollment, Node health & version
Node resource overview <- Multi-node registration / enrollment, Node health & version
Cross-node sync of files/images/SSL/apps/config <- Multi-node registration / enrollment, Node authentication & Core↔Agent mutual auth
WAF custom rules <- WAF engine install/bootstrap
WAF ACL — IP/UA/URL allow-deny + IP groups <- WAF custom rules, WAF engine install/bootstrap
WAF region / geo rules <- WAF engine install/bootstrap
WAF attack logs and records <- WAF engine install/bootstrap
WAF trends/statistics/attack map <- WAF attack logs and records, WAF region / geo rules
WAF rule/config export-import <- WAF custom rules, WAF ACL — IP/UA/URL allow-deny + IP groups
WAF custom block page <- WAF engine install/bootstrap
Website anti-tamper protection <- Website module (OSS present)
Anti-tamper protect/exclude rules <- Website anti-tamper protection
Anti-tamper audit log and recovery <- Website anti-tamper protection, Anti-tamper protect/exclude rules
Website traffic / QPS monitoring <- Website access monitoring PV/UV/IP
Website request logs analysis <- Website access monitoring PV/UV/IP
Website source/device/URI ranking <- Website access monitoring PV/UV/IP
Roles <- Users management (beyond single admin)
View vs manage permission granularity <- Roles, Users management (beyond single admin)
Node-scoped permissions / RBAC <- Users management (beyond single admin), Roles, Multi-node registration / enrollment
Operations reports <- Security scoring / assessment, monitor/WAF/SSL/cronjob/container (OSS present)
Scheduled report export <- Operations reports
Security scoring / assessment <- monitor/SSH/WAF/SSL/cronjob/container (OSS present)
Custom application repository <- app store (OSS present), Multi-node registration / enrollment
Enhanced proxy management (Docker proxy sync) <- base system proxy setting (OSS present), Multi-node registration / enrollment
Skills Hub <- AI Agents runtime (OSS present), Multi-node registration / enrollment
AI benchmark testing <- vLLM management, Model download manager
AI gateway (routing / content-audit / token-metering) <- AgentAccount/model-pool (OSS present), Model download manager
vLLM management <- Model download manager
MySQL high availability / failover <- Multi-node registration / enrollment, remote database model (OSS present)
PostgreSQL high availability <- Multi-node registration / enrollment, remote database model (OSS present)
Redis high availability <- Multi-node registration / enrollment, remote database model (OSS present)
VM networking and storage <- KVM / libvirt virtual machines
VM VNC console <- KVM / libvirt virtual machines
VM snapshots <- KVM / libvirt virtual machines, VM networking and storage
VM templates <- KVM / libvirt virtual machines, VM snapshots, VM networking and storage
Custom branding / white-label <- Open enhancement setting service (OSS present)
Enhanced custom login page <- Open enhancement setting service (OSS present), Custom branding / white-label
Advanced theme beyond v1 <- Open enhancement setting service (OSS present)
Independent SMS delivery <- Alert config/log subsystem (OSS present)
```

---
*生成：2026-07-10 · 探针核验源码 revision 508403749 · 供 v1.2+ 里程碑规划复用，不得据此宣称任何能力已实现。*
