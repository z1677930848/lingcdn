# LingCDN

LingCDN 是一套自研的 CDN/边缘加速 + WAF 平台，本仓库收录 **主控端（control plane）** 与 **节点端（edge node）** 两个核心服务。

```
┌─────────────────────────────┐        gRPC + mTLS        ┌──────────────────────┐
│  control（Go + Vue）        │ ────────────────────────► │  node × N（Rust）    │
│  - 控制面 API / Admin UI    │                            │  - HTTP/HTTPS 反代   │
│  - 节点编排 / 配置编译       │ ◄──────── 上报 / 心跳 ──── │  - 本地缓存 / WAF    │
│  - 证书 / GeoIP / 升级分发   │                            │  - 可选 XDP 加速     │
└─────────────────────────────┘                            └──────────────────────┘
```

## 目录结构

```
.
├── proto/                     # gRPC IDL（admin_control.proto / node_control.proto）
├── services/
│   ├── control/               # 主控端（Go 1.25）
│   │   ├── cmd/lingcdn-control/   # 二进制入口（serve / migrate / seed）
│   │   ├── internal/              # 业务实现：cert/compiler/dnsprovider/server…
│   │   ├── proto/gen/             # 由 buf 生成的 *.pb.go（已入库，无需 protoc）
│   │   ├── ui/                    # 控制台前端（Vue 3 + Vite + TDesign）
│   │   ├── data/geoip/            # 运行时 GeoIP 数据库（不入库，启动后自动下载）
│   │   └── config.example.yaml    # 配置模板，复制为 config.yaml 后修改
│   └── node/                  # 节点端（Rust 2021，tokio + hyper）
│       ├── src/                   # 业务实现：proxy / cache / waf / xdp …
│       ├── xdp-ebpf/              # eBPF 程序（Linux 专用，需 nightly）
│       └── node.example.toml      # 节点配置模板
├── deployment/                # 部署清单（systemd / docker-compose 等，按需补充）
├── tools/                     # 辅助脚本与工具（cross-bin 不入库）
└── build_control.sh           # 主控端打包脚本（输出 tar.gz）
```

## 构建

### 主控端（Go + Vue）

```bash
# 1) 前端
cd services/control/ui
pnpm install      # 或 npm install
pnpm build        # 产物输出到 ui/dist

# 2) 后端
cd ../..
cd services/control
go build -o bin/lingcdn-control ./cmd/lingcdn-control
```

> 国内网络环境推荐：`export GOPROXY=https://goproxy.cn,direct`

完整打包（注入版本号 + 打成 tar.gz）：

```bash
bash build_control.sh --version 1.0.7 --channel stable
```

### 节点端（Rust）

```bash
cd services/node
cargo build --release
# Linux 启用 XDP（需要 root + 内核 ≥ 5.10 + libbpf）
cargo build --release --features xdp
```

## 运行

### 1. 准备配置

```bash
# 主控
cp services/control/config.example.yaml services/control/config.yaml
# 节点
cp services/node/node.example.toml services/node/node.toml
```

按文件内注释填入 `DATABASE_URL`、`SERVICE_TOKEN`、`AUTH_SECRET`、`bootstrap_token` 等敏感值。
所有配置项均可被同名大写下划线环境变量覆盖。

### 2. 启动主控

```bash
cd services/control
./bin/lingcdn-control migrate     # 首次运行，建库 + 写入种子数据
./bin/lingcdn-control serve --config config.yaml
```

默认端口：

| 用途        | 监听     |
|-------------|----------|
| Admin HTTP  | `:8080`  |
| Node gRPC   | `:9443`  |
| Metrics     | `:9100`  |
| HTTPS（可选）| `:443`   |

### 3. 注册并启动节点

1. 在主控控制台为节点签发 `bootstrap_token` 与（可选）mTLS 证书；
2. 把 token 写入 `node.toml` 或 `BOOTSTRAP_TOKEN` 环境变量；
3. 运行：

```bash
cd services/node
./target/release/lingcdn-node
```

## 协议生成（可选）

`services/control/proto/gen` 已包含 `*.pb.go`，无需额外步骤即可 `go build`。
若需重新生成：

```bash
cd proto
buf generate
```

## 安全注意事项

- `services/control/config.yaml`、`config.local.yaml`、`data/license.json`、`ui/.env*` 已在 `.gitignore` 中显式排除。**禁止**提交真实凭证。
- 部署时建议使用：
  - `openssl rand -base64 32` 生成 `SERVICE_TOKEN` / `AUTH_SECRET` / `WEBHOOK_SECRET`；
  - 单独的最小权限 PostgreSQL 账号；
  - 节点 ↔ 主控启用 mTLS（`control_tls_*` 字段）。

## 开发约束

- Go ≥ 1.25，Rust ≥ 1.79（stable），Node ≥ 20。
- 主控前端构建产物（`ui/dist`）会被后端通过 `CONTROL_UI_DIR` 提供为静态资源。
- 节点端内置 captcha / WAF / GeoIP / 自动升级 / 可选 XDP；XDP 仅在 Linux 编译生效。

## License

本仓库为内部 / 商业项目源码，未附带开源 License。如需重新发布，请先与作者协商授权。
