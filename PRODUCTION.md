# LingCDN 生产环境检查清单

部署到生产环境前，请逐项确认：

## 必做项

- [ ] **数据库**：`STORE_BACKEND=postgres`，配置有效的 `DATABASE_URL`
- [ ] **支付**：`PAYMENT_PROVIDER=epay`（勿使用默认 `mock`）；配置易支付 PID/密钥
- [ ] **密钥**：修改 `config.yaml` 中所有 `CHANGE_ME` / 默认 token
- [ ] **HTTPS**：主控 Admin UI 与 API 走 TLS 反向代理
- [ ] **节点 mTLS**：节点与控制面 gRPC 使用有效证书
- [ ] **Elasticsearch**：配置 ES 地址；各节点部署 Filebeat 采集日志
- [ ] **授权**：激活有效 license（`/api/license/activate`）

## 推荐项

- [ ] **Redis**：配置 `REDIS_URL` 用于缓存刷新/预加载协调（连接失败会静默降级）
- [ ] **SMTP**：配置邮件发送，用于注册验证码与通知
- [ ] **GeoIP**：确认 GeoIP 数据库可自动下载或手动放置
- [ ] **备份**：PostgreSQL 定期备份；证书与配置版本归档
- [ ] **监控**：暴露 `metrics` 端口并接入 Prometheus/Grafana

## 功能边界

| 功能 | 生产可用性 |
|------|-----------|
| HTTP/HTTPS CDN + WAF | ✅ |
| L4 TCP/UDP 转发 | ✅（需套餐配额） |
| 余额充值 | ✅（需 epay） |
| 余额提现 | ✅（需管理员审核） |
| DNS 自动同步 | 阿里云 / DNSPod / Cloudflare（Route53 等 5 家可选，同步开发中） |
| DNS Recover/Cleanup | ⚠️ 仅校验凭证 |
| 多主控 HA | 不支持（单主控部署） |
| 服务端 logout | ✅（单进程 jti 黑名单） |

## 配置示例

```yaml
store_backend: postgres
database_url: postgres://user:pass@127.0.0.1:5432/lingcdn?sslmode=disable
payment_enabled: true
payment_provider: epay
payment_epay_pid: "your-pid"
payment_epay_key: "your-key"
elasticsearch_url: https://es.example.com:9200
redis_url: redis://127.0.0.1:6379/0
```

## 启动验证

```bash
# 主控
./lingcdn-control serve --config config.yaml

# 检查
curl -s http://127.0.0.1:8080/api/public/settings
curl -s http://127.0.0.1:8080/api/public/license/status
```

启动时若看到 `PAYMENT_PROVIDER=mock is active` 警告，请立即更换支付配置。
