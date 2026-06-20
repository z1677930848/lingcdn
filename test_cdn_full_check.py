#!/usr/bin/env python3
"""Full CDN feature availability check."""
import json
import sys
import time
import urllib.error
import urllib.request

BASE = "http://127.0.0.1:8080"


def req(method, path, body=None, token=None):
    url = BASE + path
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"Bearer {token}"
    data = json.dumps(body).encode() if body is not None else None
    r = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(r, timeout=30) as resp:
            raw = resp.read().decode()
            return resp.status, json.loads(raw) if raw else {}
    except urllib.error.HTTPError as e:
        raw = e.read().decode()
        try:
            return e.code, json.loads(raw) if raw else {}
        except json.JSONDecodeError:
            return e.code, {"raw": raw}


results = []


def check(name, ok, detail="", note=""):
    results.append((name, ok, detail, note))
    mark = "OK" if ok else "FAIL"
    line = f"[{mark}] {name}"
    if detail:
        line += f" - {detail}"
    if note:
        line += f" ({note})"
    print(line)


def main():
    code, data = req("POST", "/api/auth/login", {"identifier": "admin", "password": "admin123456"})
    if code != 200:
        check("Admin Login", False, f"HTTP {code} {data}")
        return 1
    token = data["token"]
    check("Admin Login", True)

    reads = [
        ("服务概览", "GET", "/api/overview"),
        ("管理统计", "GET", "/api/admin/stats"),
        ("域名列表", "GET", "/api/domains"),
        ("集群列表", "GET", "/api/clusters"),
        ("节点列表", "GET", "/api/nodes"),
        ("证书列表", "GET", "/api/certificates"),
        ("WAF 策略", "GET", "/api/waf/policies"),
        ("WAF 封禁", "GET", "/api/waf/bans"),
        ("WAF 白名单", "GET", "/api/waf/whitelist"),
        ("全局 CC", "GET", "/api/waf/cc"),
        ("缓存规则", "GET", "/api/cache-rules"),
        ("L4 转发", "GET", "/api/stream-forwards"),
        ("DNS 配置", "GET", "/api/dns/config"),
        ("DNS 任务", "GET", "/api/dns/tasks"),
        ("配置版本", "GET", "/api/config/versions"),
        ("同步状态", "GET", "/api/sync/active"),
        ("发布任务", "GET", "/api/system/publish/tasks"),
        ("系统任务", "GET", "/api/system/tasks"),
        ("系统设置", "GET", "/api/settings"),
        ("日志状态", "GET", "/api/logs/status"),
        ("License", "GET", "/api/license/status"),
        ("公开设置", "GET", "/api/public/settings"),
        ("DDoS 统计", "GET", "/api/ddos/xdp/stats"),
        ("工单(管理)", "GET", "/api/admin/tickets"),
        ("用户列表", "GET", "/api/admin/users"),
        ("产品列表", "GET", "/api/products"),
    ]
    for name, method, path in reads:
        code, _ = req(method, path, token=token)
        check(name, 200 <= code < 300, f"HTTP {code}")

    # Cluster + domain (prerequisite chain)
    code, cluster = req("POST", "/api/clusters", {"name": "health-cluster", "dns_zone": "cdn.test", "enabled": True}, token=token)
    cid = cluster.get("id", "") if isinstance(cluster, dict) else ""
    check("创建集群", 200 <= code < 300 or code == 409, f"HTTP {code}")

    if not cid:
        _, cl = req("GET", "/api/clusters", token=token)
        for c in (cl.get("clusters") or []):
            if c:
                cid = c.get("id", "")
                break

    code, domain = req(
        "POST",
        "/api/domains",
        {"name": "health.example.com", "origin_host": "1.2.3.4", "origin_port": 80, "line_group_id": cid},
        token=token,
    )
    did = domain.get("id", "") if isinstance(domain, dict) else ""
    check("创建域名", 200 <= code < 300 or code == 409, f"HTTP {code}")

    if did:
        code, sec = req(
            "PUT",
            f"/api/domains/{did}/security",
            {"default_mode": "slide", "ban_seconds": 300, "fail_limit": 3},
            token=token,
        )
        check("域名安全(滑块验证)", 200 <= code < 300, f"HTTP {code} mode={sec.get('default_mode','')}")

    # WAF
    code, waf = req(
        "POST",
        "/api/waf/policies",
        {
            "name": "health-waf",
            "scope": "global",
            "enabled": True,
            "rules": [{"type": "rate_limit", "action": "deny", "threshold": 100, "window_seconds": 60, "enabled": True}],
        },
        token=token,
    )
    wid = waf.get("id", "") if isinstance(waf, dict) else ""
    check("创建 WAF 策略", 200 <= code < 300, f"HTTP {code}")

    if wid:
        code, _ = req("PATCH", f"/api/waf/policies/{wid}/enabled", {"enabled": False}, token=token)
        check("WAF 策略开关", 200 <= code < 300, f"HTTP {code}")
        code, _ = req("PATCH", f"/api/waf/policies/{wid}/enabled", {"enabled": True}, token=token)
        check("WAF 策略恢复", 200 <= code < 300, f"HTTP {code}")

    code, _ = req("POST", "/api/waf/rulesets/owasp_common", {}, token=token)
    check("OWASP 规则集", 200 <= code < 300, f"HTTP {code}")

    code, _ = req("POST", "/api/config/publish", {}, token=token)
    check("配置发布", 200 <= code < 300, f"HTTP {code}")

    code, _ = req("POST", "/api/dns/sync", {}, token=token)
    check("DNS 同步触发", 200 <= code < 300, f"HTTP {code}")

    code, purge = req("POST", "/api/purge", {"urls": ["https://health.example.com/"]}, token=token)
    check(
        "缓存刷新",
        code == 200 or (code == 500 and "no nodes" in str(purge)),
        f"HTTP {code}",
        "无节点时预期失败" if code == 500 else "",
    )

    code, preload = req("POST", "/api/preload", {"urls": ["https://health.example.com/"]}, token=token)
    check(
        "缓存预热",
        code == 200 or (code == 500 and "no nodes" in str(preload)),
        f"HTTP {code}",
        "无节点时预期失败" if code == 500 else "",
    )

    code, cache = req("POST", "/api/cache-rules", {"name": "health-cache", "path_pattern": "*.jpg", "ttl_seconds": 3600, "enabled": True}, token=token)
    check("创建缓存规则", 200 <= code < 300, f"HTTP {code}")

    code, cc = req(
        "POST",
        "/api/waf/cc",
        {"level": "medium", "action": "challenge", "captcha_type": "slide", "ban_seconds": 300, "fail_limit": 3},
        token=token,
    )
    check("全局 CC(滑块)", 200 <= code < 300, f"HTTP {code}")

    code, l4 = req(
        "POST",
        "/api/stream-forwards",
        {"name": "health-l4", "listen_port": 19090, "origin_host": "1.2.3.4", "origin_port": 80, "protocol": "tcp", "enabled": True},
        token=token,
    )
    check("L4 转发", 200 <= code < 300, f"HTTP {code} {l4.get('error','')}", "需套餐配额" if code == 403 else "")

    time.sleep(1)
    code, tasks = req("GET", "/api/system/publish/tasks", token=token)
    n = len(tasks.get("tasks") or [])
    check("发布任务历史", code == 200, f"count={n}")

    ok = sum(1 for _, s, _, _ in results if s)
    fail = len(results) - ok
    print(f"\n=== TOTAL {len(results)} | OK {ok} | FAIL {fail} ===")
    if fail:
        print("\nFailed items:")
        for name, s, detail, note in results:
            if not s:
                print(f"  - {name}: {detail} {note}".strip())
    return 0 if fail == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
