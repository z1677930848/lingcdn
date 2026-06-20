# LingCDN docker-compose 开发/演示栈

> 一键启动 PostgreSQL + Redis + Elasticsearch + 主控 + 节点 + 日志采集。
> 生产环境请参考 `PRODUCTION.md` 与 `deployment/` 下的 systemd 清单。

## 启动

```bash
cd cdn系统程序
docker compose up -d --build
```

首次构建节点镜像（Rust）可能需要 10–20 分钟。

## 服务

| 服务 | 端口 | 用途 |
|------|------|------|
| postgres | 5432 | 主控数据库 |
| redis | 6379 | 缓存 / JWT |
| elasticsearch | 9200 | 访问日志与监控 |
| control | 8080 / 9443 / 9100 | 主控 HTTP / gRPC / Metrics |
| node | 8088 / 9101 | CDN 节点 HTTP / Ops |
| filebeat | — | 采集节点 access.log / error.log 写入 ES |

## 访问

- 管理后台：http://127.0.0.1:8080/nimda （admin / admin123456）
- 访问日志：http://127.0.0.1:8080/admin/dashboard/access-logs
- ES 健康：http://127.0.0.1:9200/_cluster/health

主控容器内 ES 地址为 `http://elasticsearch:9200`（见 `services/control/config.docker.yaml`）。

## Windows 快捷脚本

Docker Desktop 就绪后：

```bat
d:\lingcdn\tools\start_es_docker.cmd
```

仅启动 ES + 配置主控 + 导入本地日志（主控/节点仍在宿主机运行时）。

完整容器栈：

```bat
cd /d d:\lingcdn\cdn系统程序
docker compose up -d --build
```

## 停止

```bash
docker compose down
```

数据卷会保留；需要清空时加 `-v`。
