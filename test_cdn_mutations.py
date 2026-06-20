#!/usr/bin/env python3
"""Extended CDN mutation tests with proper payloads."""
import json
import sys
import time
import urllib.error
import urllib.request

BASE = "http://127.0.0.1:8080"


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
            return resp.status, json.loads(raw) if raw else {}
    except urllib.error.HTTPError as e:
        raw = e.read().decode()
        try:
            return e.code, json.loads(raw) if raw else {}
        except json.JSONDecodeError:
            return e.code, {"raw": raw}


def main():
    code, data = req("POST", "/api/auth/login", {"identifier": "admin", "password": "admin123456"})
    if code != 200:
        print(f"LOGIN FAIL: {code} {data}")
        return 1
    token = data["token"]
    print("[PASS] Admin Login")

    # Create cluster (required for domains)
    code, cluster = req(
        "POST",
        "/api/clusters",
        {"name": "test-cluster", "dns_zone": "cdn.local.test", "enabled": True},
        token=token,
    )
    cluster_id = cluster.get("id", "") if isinstance(cluster, dict) else ""
    print(f"[{'PASS' if 200 <= code < 300 else 'FAIL'}] Create Cluster HTTP {code} id={cluster_id} err={cluster.get('error','')}")

    # Create domain
    code, domain = req(
        "POST",
        "/api/domains",
        {
            "name": "test-check.example.com",
            "origin_host": "1.2.3.4",
            "origin_port": 80,
            "line_group_id": cluster_id,
        },
        token=token,
    )
    domain_id = domain.get("id", "") if isinstance(domain, dict) else ""
    print(f"[{'PASS' if 200 <= code < 300 else 'FAIL'}] Create Domain HTTP {code} id={domain_id} err={domain.get('error','')}")

    # WAF policy with valid rule type
    code, waf = req(
        "POST",
        "/api/waf/policies",
        {
            "name": "test-waf-policy",
            "scope": "global",
            "enabled": True,
            "rules": [
                {
                    "type": "rate_limit",
                    "action": "deny",
                    "threshold": 100,
                    "window_seconds": 60,
                    "enabled": True,
                }
            ],
        },
        token=token,
    )
    waf_id = waf.get("id", "") if isinstance(waf, dict) else ""
    print(f"[{'PASS' if 200 <= code < 300 else 'FAIL'}] Create WAF Policy HTTP {code} id={waf_id} err={waf.get('error','')}")

    if waf_id:
        code, _ = req("PATCH", f"/api/waf/policies/{waf_id}", {"enabled": False}, token=token)
        print(f"[{'PASS' if 200 <= code < 300 else 'FAIL'}] WAF Disable HTTP {code}")
        code, _ = req("PATCH", f"/api/waf/policies/{waf_id}", {"enabled": True}, token=token)
        print(f"[{'PASS' if 200 <= code < 300 else 'FAIL'}] WAF Enable HTTP {code}")

    # Apply OWASP ruleset
    code, ruleset = req("POST", "/api/waf/rulesets/owasp", {}, token=token)
    print(f"[{'PASS' if 200 <= code < 300 else 'FAIL'}] Apply OWASP Ruleset HTTP {code} {ruleset.get('error','') or ruleset.get('message','')}")

    # Config publish
    code, pub = req("POST", "/api/config/publish", {}, token=token)
    print(f"[{'PASS' if 200 <= code < 300 else 'FAIL'}] Config Publish HTTP {code} {pub}")

    # L4 forward
    code, l4 = req(
        "POST",
        "/api/stream-forwards",
        {
            "name": "test-tcp",
            "listen_port": 18080,
            "origin_host": "1.2.3.4",
            "origin_port": 80,
            "protocol": "tcp",
            "enabled": True,
        },
        token=token,
    )
    print(f"[{'PASS' if 200 <= code < 300 else 'FAIL'}] Create L4 HTTP {code} err={l4.get('error','')}")

    # Purge (expected fail without nodes)
    code, purge = req("POST", "/api/purge", {"urls": ["https://test-check.example.com/"]}, token=token)
    purge_ok = code == 200 or (code == 500 and "no nodes" in str(purge))
    print(f"[{'PASS' if purge_ok else 'FAIL'}] Cache Purge HTTP {code} {purge} (no-node expected in local)")

    # DNS sync
    code, dns = req("POST", "/api/dns/sync", {}, token=token)
    print(f"[{'PASS' if 200 <= code < 300 else 'FAIL'}] DNS Sync HTTP {code} {dns.get('error','') or dns.get('message','')}")

    time.sleep(2)
    code, sync = req("GET", "/api/sync/active", token=token)
    print(f"[{'PASS' if code == 200 else 'FAIL'}] Sync Active HTTP {code} {sync}")

    code, tasks = req("GET", "/api/system/publish/tasks", token=token)
    count = len(tasks.get("tasks") or tasks.get("items") or [])
    print(f"[{'PASS' if code == 200 else 'FAIL'}] Publish Tasks HTTP {code} count={count}")

    return 0


if __name__ == "__main__":
    sys.exit(main())
