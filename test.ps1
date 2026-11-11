$ErrorActionPreference = "Stop"
$port = 8787
$token = "test-token-for-local"
$bin = Join-Path $env:TEMP "claude-router-test.exe"

go build -o $bin .

$configJson = @"
{
  "listen": "127.0.0.1:$port",
  "router_token": "$token",
  "default_cooldown_seconds": 60,
  "providers": [
    {
      "name": "openrouter",
      "base_url": "https://openrouter.ai/api",
      "auth_style": "bearer",
      "tier": 0,
      "override_model": "recursive/flint-v1.0:free",
      "keys": ["sk-or-v1-test"]
    }
  ]
}
"@

$tmp = Join-Path $env:TEMP "cc-router-test"
New-Item -ItemType Directory -Path $tmp -Force | Out-Null
$configPath = Join-Path $tmp "config.json"
Set-Content -Path $configPath -Value $configJson -NoNewline

$env:CC_ROUTER_CONFIG = $configPath
$env:ROUTER_TOKEN = $token

$proc = Start-Process -FilePath $bin -PassThru -NoNewWindow

Start-Sleep -Seconds 8

try {
  $health = Invoke-WebRequest -Uri "http://127.0.0.1:$port/api/hello" -Method Get -UseBasicParsing
  Write-Host "health: $($health.StatusCode)"
} catch {
  Write-Host "health: FAILED"
}

try {
  $resp = Invoke-WebRequest -Uri "http://127.0.0.1:$port/v1/messages/count_tokens" -Method Post -UseBasicParsing -Body '{"model":"test"}' -ContentType "application/json"
  Write-Host "unauth_count_tokens: $($resp.StatusCode)"
} catch {
  Write-Host "unauth_count_tokens: FAILED"
}

if ($proc) { Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue }

if ($health.StatusCode -eq 200 -and $resp.StatusCode -eq 401) {
  Write-Host "TESTS_PASSED"
  exit 0
} else {
  Write-Host "TESTS_FAILED"
  exit 1
}
