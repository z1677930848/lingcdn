#!/usr/bin/env python3
"""CDN control plane full API test with admin token."""
import json
import os
import sys
import time
import urllib.error
import urllib.request

BASE = "http://127.0.0.1:8080"
TOKEN = os.environ.get("ADMIN_TOKEN", "")


def req(method, path, body=None, token=None):
    url = BASE + path
    data = None
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"Bearer {token}"
    if body is not None:
        data = json.dumps(body).encode()
    r = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(r, timeout=30) as resp:
            raw = resp.read().decode()
            try:
                parsed = json.loads(raw) if raw else {}
            except json.JSONDecodeError:
                parsed = raw
            return resp.status, parsed
    except urllib.error.HTTPError as e:
        raw = e.read().decode()
        try:
            parsed = json.loads(raw) if raw else {}
        except json.JSONDecodeError:
            parsed = raw
        return e.code, parsed


results = []


def check(name, ok, detail=""):
    results.append((name, ok, detail))
    mark = "PASS" if ok else "FAIL"
    print(f"[{mark}] {name}" + (f" - {detail}" if detail else ""))


def main():
    token = TOKEN
    if not token:
        code, data = req("POST", "/api/auth/login", {"identifier": "admin", "password": "admin123456"})
        if code == 200 and isinstance(data, dict) and data.get("token"):
            token = data["token"]
            check("Admin Login", True)
        else:
            check("Admin Login", False, f"HTTP {code} {data}")
            return 1

    read_tests = [
        ("Overview", "GET", "/api/overview"),
        ("Domains List", "GET", "/api/domains"),
        ("Sync Active", "GET", "/api/sync/active"),
        ("Config Versions", "GET", "/api/config/versions"),
        ("WAF Policies", "GET", "/api/waf/policies"),
        ("WAF Bans", "GET", "/api/waf/bans"),
        ("WAF Whitelist", "GET", "/api/waf/whitelist"),
        ("Cache Rules", "GET", "/api/cache-rules"),
        ("DNS Config", "GET", "/api/dns/config"),
        ("DNS Tasks", "GET", "/api/dns/tasks"),
        ("Nodes", "GET", "/api/nodes"),
        ("Clusters", "GET", "/api/clusters"),
        ("L4 Forwards", "GET", "/api/stream-forwards"),
        ("Admin L4", "GET", "/api/admin/stream-forwards"),
        ("Certificates", "GET", "/api/certificates"),
        ("System Tasks", "GET", "/api/system/tasks"),
        ("Publish Tasks", "GET", "/api/system/publish/tasks"),
        ("DDoS XDP Stats", "GET", "/api/ddos/xdp/stats"),
        ("Settings", "GET", "/api/settings"),
        ("Logs Status", "GET", "/api/logs/status"),
        ("Admin Stats", "GET", "/api/admin/stats"),
        ("License Status", "GET", "/api/license/status"),
    ]

    for name, method, path in read_tests:
        code, data = req(method, path, token=token)
        check(name, 200 <= code < 300, f"HTTP {code}")

    # Create domain
    code, domain = req(
        "POST",
        "/api/domains",
        {"domain": "test-check.example.com", "origin": "1.2.3.4"},
        token=token,
    )
    domain_id = domain.get("id", "") if isinstance(domain, dict) else ""
    check("Create Domain", 200 <= code < 300, f"HTTP {code} id={domain_id}")

    # Create WAF policy
    code, waf = req(
        "POST",
        "/api/waf/policies",
        {
            "name": "auto-test-policy",
            "scope": "global",
            "enabled": True,
            "rules": [{"type": "sqli", "action": "block", "enabled": True}],
        },
        token=token,
    )
    waf_id = waf.get("id", "") if isinstance(waf, dict) else ""
    check("Create WAF Policy", 200 <= code < 300, f"HTTP {code} id={waf_id}")

    # WAF toggle off
    if waf_id:
        code, _ = req("PATCH", f"/api/waf/policies/{waf_id}", {"enabled": False}, token=token)
        check("WAF Policy Disable", 200 <= code < 300, f"HTTP {code}")
        code, _ = req("PATCH", f"/api/waf/policies/{waf_id}", {"enabled": True}, token=token)
        check("WAF Policy Enable", 200 <= code < 300, f"HTTP {code}")

    # Cache rule
    code, cache = req(
        "POST",
        "/api/cache-rules",
        {"name": "test-rule", "pattern": "*.jpg", "ttl": 3600, "enabled": True},
        token=token,
    )
    check("Create Cache Rule", 200 <= code < 300, f"HTTP {code}")

    # Config publish
    code, pub = req("POST", "/api/config/publish", {}, token=token)
    pub_ok = 200 <= code < 300
    pub_detail = f"HTTP {code}"
    if isinstance(pub, dict):
        pub_detail += f" task={pub.get('id') or pub.get('task_id') or pub}"
    check("Config Publish (Manual)", pub_ok, pub_detail)

    # Purge
    code, purge = req(
        "POST",
        "/api/purge",
        {"urls": ["https://test-check.example.com/index.html"]},
        token=token,
    )
    check("Cache Purge", 200 <= code < 300, f"HTTP {code} {purge if isinstance(purge, dict) else ''}")

    # Preload
    code, preload = req(
        "POST",
        "/api/preload",
        {"urls": ["https://test-check.example.com/index.html"]},
        token=token,
    )
    check("Cache Preload", 200 <= code < 300, f"HTTP {code}")

    # DNS sync (may fail without provider config - that's expected)
    code, dns = req("POST", "/api/dns/sync", {}, token=token)
    dns_ok = 200 <= code < 300 or (code == 400 and isinstance(dns, dict))
    check("DNS Sync Trigger", dns_ok, f"HTTP {code} {dns.get('error','') if isinstance(dns,dict) else dns}")

    # L4 forward
    code, l4 = req(
        "POST",
        "/api/stream-forwards",
        {"name": "test-tcp", "listen_port": 18080, "backend_host": "1.2.3.4", "backend_port": 80, "protocol": "tcp"},
        token=token,
    )
    check("Create L4 Forward", 200 <= code < 300, f"HTTP {code}")

    time.sleep(2)
    code, sync = req("GET", "/api/sync/active", token=token)
    sync_detail = f"HTTP {code}"
    if isinstance(sync, dict):
        tasks = sync.get("tasks") or sync.get("active") or sync
        sync_detail += f" active={json.dumps(tasks, ensure_ascii=False)[:200]}"
    check("Sync Active After Mutations", code == 200, sync_detail)

    code, pub_tasks = req("GET", "/api/system/publish/tasks", token=token)
    if code == 200 and isinstance(pub_tasks, dict):
        tasks = pub_tasks.get("tasks") or pub_tasks.get("items") or []
        check("Publish Task History", len(tasks) >= 0, f"count={len(tasks)}")
    else:
        check("Publish Task History", False, f"HTTP {code}")

    passed = sum(1 for _, ok, _ in results if ok)
    failed = sum(1 for _, ok, _ in results if not ok)
    print(f"\n=== SUMMARY: PASS={passed} FAIL={failed} TOTAL={len(results)} ===")
    return 0 if failed == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
