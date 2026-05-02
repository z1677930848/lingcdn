# Control UI / Go 后端集成说明

本文档描述 `services/control/ui` 与 `services/control/internal/server` 的接口适配关系。

## 1. 基础约定
- 前端默认请求同源 `/api/*`
- 前端通过 `src/lib/api.ts` 统一发起请求
- 后端路由由 `internal/server/server.go` 的 `adminMux()` 注册

## 2. 当前可用接口（实际存在）

### 认证与公共
- `POST /api/auth/login`
- `POST /api/auth/register`
- `POST /api/auth/register/email/request`
- `GET /api/auth/captcha`
- `GET /api/auth/me`
- `GET /api/public/settings`
- `GET /api/public/announcements`
- `GET /api/public/license/status`

### 用户侧核心
- `GET /api/products`
- `GET /api/products/:id/builds`
- `GET /api/overview`

### 管理侧与系统
- `GET|POST|PATCH|DELETE /api/admin/*`
- `GET|POST|PATCH|DELETE /api/nodes*`
- `GET|POST|PATCH|DELETE /api/domains*`
- `GET|POST /api/license/status`
- `POST /api/license/activate`
- `GET|POST /api/system/upgrade*`
- 以及其他 `adminMux()` 中已注册接口

## 3. 当前未开放接口（前端已禁用）
- `POST /api/auth/logout`
- `POST /api/auth/password/reset/request`
- `POST /api/auth/password/reset/confirm`
- `GET|POST /api/licenses`
- `POST /api/licenses/verify`
- `POST /api/downloads/ticket`
- `GET /api/balance`
- `GET /api/balance/transactions`
- `GET|POST /api/balance/recharges`
- `GET|POST /api/balance/withdrawals`

对应页面行为：
- 保留页面入口
- 首屏不发起上述不存在请求
- 显示统一提示“后端暂未开放该能力”
- 表单与按钮禁用，避免误触发

## 4. 后端 fallback 行为
已修正：
- 未匹配的 `/api/*` 路径返回 `404 + application/json`
- 非 `/api/*` 路径仍保留 SPA 回退（存在 UI 构建产物时返回 `index.html`）

## 5. portal 上游来源
当前为强制固定：
- 上游统一固定为 `https://auth.lingcdn.cloud`
- 适用范围：license verify、system report、upgrade source、upgrade scripts
- 代码已标注“非强制情况下不准修改”

## 6. 升级脚本链路（拆分）
- 主控升级脚本：`GET /control_update.sh`
- 节点升级脚本：`GET /node_update.sh`
- `POST /api/system/upgrade/control` 会执行 `control_update.sh`
- 节点心跳领取升级命令后，节点进程执行 `node_update.sh`
- 两脚本都要求下载地址是 `https://auth.lingcdn.cloud/*`，否则拒绝执行

## 7. 调试建议
- 前端调试：`npm run dev`
- 前端校验：`npm run lint && npm run build`
- 后端重点测试：`go test ./internal/server -run "Fallback|Upgrade|Report|License"`
