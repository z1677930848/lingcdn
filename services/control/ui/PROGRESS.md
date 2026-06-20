# 前后端对接进度报告

> 更新时间：2026-06-19

## ✅ 已完成

### 工单系统
- 用户：新建 / 列表 / 详情 / 回复 / 关闭（`/dashboard/tickets`）
- 管理：列表筛选 / 详情 / 回复 / 状态与优先级（`/admin/dashboard/tickets`）
- API：`/api/tickets*`、`/api/admin/tickets*`
- 通知：用户新建/回复工单 → Webhook（`notify_ticket_reply`）；管理员回复 → 邮件（需 SMTP）

### 认证与用户
- 登录 / 注册 / 找回密码；`/api/auth/me` 含分组 permissions
- Logout 进程内 jti 黑名单（单主控）

### 域名与 CDN、余额、L4、DNS、升级等
- 见历史版本说明；核心链路可用

### 部署
- `docker-compose.yml`：PostgreSQL + Redis + Elasticsearch 本地栈

## ⏳ 部分完成

| 功能 | 状态 |
|------|------|
| DNS 扩展提供商 | Route53 等 5 家可选，自动同步未实现 |
| 用户分组 | 仅归类；权限项预留，不限制用户功能 |
| 无 ES 时统计 | 仪表盘/监控/域名健康返回 `demo_data` 并提示 |
| 访问日志 | 用户侧 `/api/logs/status` + 按名下域名 scoped 搜索 |

## ❌ 未实现

- 内置 HTTP 日志 ingest（Filebeat 方案）
- Route53 等 DNS 记录同步 API
- 多主控 HA（**不在产品范围**）

## 调试

```bash
cd services/control/ui && npm run dev
cd services/control && go run ./cmd/lingcdn-control serve --config config.local.yaml
go test ./internal/server ./internal/compiler -count=1
```
