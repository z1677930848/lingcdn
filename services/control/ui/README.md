# LingCDN Control UI

LingCDN `control` 子系统的前端控制台，技术栈为 `Vue 3 + Vite + TypeScript + Pinia + Vue Router + TDesign`。

## 目录与构建
- 源码目录：`src`
- 构建工具：`vite`
- 开发端口：`5174`
- 构建输出：`dist`

## 启动与构建
```bash
npm install
npm run dev
npm run lint
npm run build
```

## 环境变量
- `VITE_API_BASE`
  - 默认空字符串，走同源 `/api/*`
  - 设置为 `auto` 时也等价于空字符串
  - 可设置为完整后端地址（例如 `http://127.0.0.1:8080`）

## 当前后端能力边界（与 Go `control` 对齐）
已对齐并可用的主要接口：
- 认证与会话：`/api/auth/login`、`/api/auth/register`、`/api/auth/me`
- 公共信息：`/api/public/settings`、`/api/public/announcements`、`/api/public/license/status`
- 用户侧核心：`/api/products`、`/api/products/:id/builds`
- 管理侧核心：`/api/admin/*`、`/api/nodes*`、`/api/domains*`、`/api/system/*`、`/api/license/status`

当前后端未开放（前端已做禁用提示，不再触发真实请求）：
- `/api/auth/logout`
- `/api/auth/password/reset/*`
- `/api/licenses*`
- `/api/downloads/ticket`
- `/api/balance*`

## 注意事项
- 本项目已移除历史 `legacy Next 前端目录` 目录，仅保留 Vue/Vite 实现。
- 用户侧“授权/下载凭据/余额/找回密码”页面保留入口，但处于“后端暂未开放该能力”的禁用态。

