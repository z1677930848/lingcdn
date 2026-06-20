$ErrorActionPreference = "Continue"
$base = "http://127.0.0.1:8080"
$results = @()

function Add-Result($name, $ok, $detail = "") {
    $script:results += [PSCustomObject]@{ Name = $name; OK = $ok; Detail = $detail }
}

# Login
try {
    $loginBody = @{ identifier = "admin"; password = "admin123456" } | ConvertTo-Json
    $loginResp = Invoke-RestMethod -Uri "$base/api/auth/login" -Method POST -Body $loginBody -ContentType "application/json"
    $token = $loginResp.token
    Add-Result "Admin Login" $true "token ok"
} catch {
    Add-Result "Admin Login" $false $_.Exception.Message
    $results | Format-Table -AutoSize
    exit 1
}

$headers = @{ Authorization = "Bearer $token" }

function Test-Get($name, $path) {
    try {
        $r = Invoke-WebRequest -Uri "$base$path" -Headers $headers -UseBasicParsing -TimeoutSec 15
        Add-Result $name $true "HTTP $($r.StatusCode)"
    } catch {
        $code = if ($_.Exception.Response) { $_.Exception.Response.StatusCode.value__ } else { "err" }
        Add-Result $name $false "HTTP $code"
    }
}

function Test-Post($name, $path, $body) {
    try {
        $json = $body | ConvertTo-Json -Depth 10 -Compress
        $r = Invoke-RestMethod -Uri "$base$path" -Method POST -Headers $headers -Body $json -ContentType "application/json" -TimeoutSec 30
        Add-Result $name $true ($r | ConvertTo-Json -Compress -Depth 3)
    } catch {
        $code = if ($_.Exception.Response) { $_.Exception.Response.StatusCode.value__ } else { "err" }
        $msg = $_.ErrorDetails.Message
        Add-Result $name $false "HTTP $code $msg"
    }
}

# Read endpoints
Test-Get "Overview" "/api/overview"
Test-Get "Domains List" "/api/domains"
Test-Get "Sync Active" "/api/sync/active"
Test-Get "Config Versions" "/api/config/versions"
Test-Get "WAF Policies" "/api/waf/policies"
Test-Get "WAF Bans" "/api/waf/bans"
Test-Get "WAF Whitelist" "/api/waf/whitelist"
Test-Get "Cache Rules" "/api/cache-rules"
Test-Get "DNS Config" "/api/dns/config"
Test-Get "DNS Tasks" "/api/dns/tasks"
Test-Get "Nodes" "/api/nodes"
Test-Get "Clusters" "/api/clusters"
Test-Get "L4 Forwards" "/api/stream-forwards"
Test-Get "Certificates" "/api/certificates"
Test-Get "System Tasks" "/api/system/tasks"
Test-Get "Publish Tasks" "/api/system/publish/tasks"
Test-Get "DDoS XDP Stats" "/api/ddos/xdp/stats"
Test-Get "Settings" "/api/settings"
Test-Get "Logs Status" "/api/logs/status"
Test-Get "Admin Stats" "/api/admin/stats"
Test-Get "License Status" "/api/license/status"

# Mutation tests
Test-Post "Create Domain" "/api/domains" @{ domain = "test-check.example.com"; origin = "1.2.3.4" }
Test-Post "Create WAF Policy" "/api/waf/policies" @{ name = "auto-test-policy"; scope = "global"; enabled = $true; rules = @(@{ type = "sqli"; action = "block"; enabled = $true }) }
Test-Post "Config Publish" "/api/config/publish" @{}
Test-Post "Cache Purge" "/api/purge" @{ urls = @("https://test-check.example.com/index.html") }
Test-Post "DNS Sync" "/api/dns/sync" @{}

# WAF policy toggle test
try {
    $policies = Invoke-RestMethod -Uri "$base/api/waf/policies" -Headers $headers
    if ($policies.policies -and $policies.policies.Count -gt 0) {
        $pid = $policies.policies[0].id
        $enabled = -not [bool]$policies.policies[0].enabled
        $toggleBody = @{ enabled = $enabled } | ConvertTo-Json
        $r = Invoke-RestMethod -Uri "$base/api/waf/policies/$pid" -Method PATCH -Headers $headers -Body $toggleBody -ContentType "application/json"
        Add-Result "WAF Policy Toggle" $true "policy=$pid enabled=$enabled"
    } else {
        Add-Result "WAF Policy Toggle" $false "no policies"
    }
} catch {
    Add-Result "WAF Policy Toggle" $false $_.Exception.Message
}

# Check sync after mutations
Start-Sleep -Seconds 2
Test-Get "Sync Active After Mutations" "/api/sync/active"

Write-Host ""
Write-Host "=== CDN API Test Results ==="
$results | Format-Table -AutoSize
$pass = ($results | Where-Object { $_.OK }).Count
$fail = ($results | Where-Object { -not $_.OK }).Count
Write-Host "PASS: $pass  FAIL: $fail  TOTAL: $($results.Count)"
