# LingCDN Control UI

LingCDN `control` 子系统的前端控制台，技术栈为 `Vue 3 + Vite + TypeScript + Pinia + Vue Router + TDesign`。

## 目录与构建
- 源码目录：`src`
- 构建工具：`vite`
- 开发端口：`5174`（绑定 `127.0.0.1`）
- 构建输出：`dist`

## 启动与构建

本地开发请同时启动 **Go 后端** 与 **Vite 前端**：

```bash
# 终端 1：后端（示例使用 config.local.yaml）
cd ../
./bin/lingcdn-control serve --config config.local.yaml

# 终端 2：前端
npm install
npm run dev
```

浏览器访问：**http://127.0.0.1:5174**

- 用户登录：`/login`
- 管理员登录：`/nimda`（不在用户登录页展示入口，避免暴露管理面）
- 后端 `:8080` 仅在 `npm run build` 后通过 `ui/dist` 提供静态资源；日常开发请使用 `:5174`

```bash
npm run lint
npm run build
```

## 环境变量
- `VITE_API_BASE`
  - 默认空字符串，走同源 `/api/*`（开发时由 Vite 代理到 `http://127.0.0.1:8080`）
  - 设置为 `auto` 时也等价于空字符串
  - 可设置为完整后端地址（例如 `http://127.0.0.1:8080`）

## 认证与验证码
- 登录 / 注册首次提交 **不显示** 图形验证码；若后端拒绝（含未带验证码），前端会 **自动展开验证码** 并提示「请完成下方验证码后再次登录」
- 验证码状态按页面分别保存在 `localStorage`（`login_*` / `admin_login_*` / `register_*`）

## 当前后端能力边界（与 Go `control` 对齐）

已对齐并可用的主要接口：
- 认证与会话：`/api/auth/login`、`/api/auth/register`、`/api/auth/logout`、`/api/auth/me`、`/api/auth/password/reset/*`
- 公共信息：`/api/public/settings`、`/api/public/announcements`、`/api/public/license/status`
- 用户侧核心：`/api/products`、`/api/overview`、`/api/balance/*`、`/api/stream-forwards`
- 管理侧核心：`/api/admin/*`、`/api/nodes*`、`/api/domains*`、`/api/system/*`、`/api/license/status`

仍在建设中的 UI 功能：
- 只读页面（监控/日志）暂未按分组权限单独隐藏

## 注意事项
- 本项目为 Vue 3 + Vite 实现（已移除历史 Next.js 前端）。
- 详细接口列表见 `INTEGRATION.md`。
