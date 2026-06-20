# Control UI / Go 后端集成说明

本文档描述 `services/control/ui` 与 `services/control/internal/server` 的接口适配关系。

## 1. 基础约定
- 前端默认请求同源 `/api/*`
- 前端通过 `src/lib/api.ts` 统一发起请求
- 后端路由由 `internal/server/router.go` 的 `adminMux()` 注册

## 2. 当前可用接口（实际存在）

### 认证与公共
- `POST /api/auth/login`
- `POST /api/auth/register`
- `POST /api/auth/register/email/request`
- `POST /api/auth/logout`
- `GET /api/auth/captcha`
- `GET /api/auth/me`
- `POST /api/auth/password/change`
- `POST /api/auth/password/reset/request`
- `POST /api/auth/password/reset/confirm`
- `GET /api/public/settings`
- `GET /api/public/announcements`
- `GET /api/public/license/status`

### 用户侧核心
- `GET|POST /api/tickets` — 工单列表 / 新建（`subject`, `category`, `body`）
- `GET /api/tickets/{id}` — 详情 + 回复列表
- `POST /api/tickets/{id}/replies` — 用户回复（已关闭不可回复）
- `POST /api/tickets/{id}/close` — 关闭工单
- `GET /api/products`
- `GET /api/products/:id/builds`
- `GET /api/overview`
- `GET /api/balance/account`
- `GET /api/balance/transactions`
- `GET|POST /api/balance/recharges`
- `GET|POST /api/balance/withdrawals`
- `GET|POST|PUT|DELETE /api/stream-forwards`（L4 四层转发）
- `GET|POST|PATCH|DELETE /api/domains*`
- `GET /api/logs/status`、`POST /api/logs/search`（用户仅可查名下域名）

### 管理侧与系统
- `GET /api/admin/tickets` — 工单列表（`status`, `user_id`, 分页）
- `GET|PATCH /api/admin/tickets/{id}` — 详情 / 更新状态与优先级
- `POST /api/admin/tickets/{id}/replies` — 管理员回复（已关闭工单会自动 reopen）
- `GET|POST|PATCH|DELETE /api/admin/*`
- `GET|POST|PATCH|DELETE /api/nodes*`
- `GET /api/admin/stream-forwards`
- `GET|POST /api/license/status`
- `POST /api/license/activate`
- `GET|POST /api/system/upgrade*`
- 以及其他 `adminMux()` 中已注册接口

## 3. 当前未开放 / 客户端降级

| 接口 | 说明 |
|------|------|
| `GET|POST /api/licenses` | 授权码管理在独立授权站，主控仅 `license/status` / `activate` |
| `POST /api/downloads/ticket` | 下载凭据由授权站处理 |

> `POST /api/auth/logout` 已实现（单主控进程内 jti 黑名单）。

## 4. 后端 fallback 行为
- 未匹配的 `/api/*` 路径返回 `404 + application/json`
- 非 `/api/*` 路径仍保留 SPA 回退（存在 UI 构建产物时返回 `index.html`）

## 5. portal 上游来源
当前为强制固定：
- 上游统一固定为 `https://auth.lingcdn.cloud`
- 适用范围：license verify、system report、upgrade source、upgrade scripts
- 代码已标注「非强制情况下不准修改」

## 6. 升级脚本链路（拆分）
- 主控升级脚本：`GET /control_update.sh`
- 节点升级脚本：`GET /node_update.sh`
- `POST /api/system/upgrade/control` 会执行 `control_update.sh`
- 节点心跳领取升级命令后，节点进程执行 `node_update.sh`
- 两脚本都要求下载地址是 `https://auth.lingcdn.cloud/*`，否则拒绝执行

## 7. 日志采集
- 节点写本地 `/var/log/lingcdn/access.log` 与 `error.log`
- 通过 Filebeat 直推 Elasticsearch（索引 `cdn-access-*` / `cdn-error-*`）
- gRPC `ReportLogs` 已废弃，不再使用内置 HTTP ingest

## 8. 调试建议
- 前端调试：`npm run dev` → http://127.0.0.1:5174（API 代理至后端 `:8080`）
- 管理员入口：`/nimda`
- 前端校验：`npm run lint && npm run build`
- 后端测试：`go test ./internal/server ./internal/compiler -count=1`
