$repo = "haidaralif-24/cc-router"
$version = if ($env:VERSION) { $env:VERSION } else { "latest" }
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
$asset = "claude-router_windows_${arch}.zip"
$url = "https://github.com/$repo/releases/$version/download/$asset"

$tmp = Join-Path $env:TEMP "claude-router-install"
if (Test-Path $tmp) { Remove-Item -Recurse -Force $tmp }
New-Item -ItemType Directory -Path $tmp | Out-Null
Invoke-WebRequest -Uri $url -OutFile (Join-Path $tmp "claude-router.zip")
Expand-Archive -Path (Join-Path $tmp "claude-router.zip") -DestinationPath $tmp -Force
$bin = Join-Path $tmp "claude-router.exe"
$destDir = if ($env:ProgramFiles) { $env:ProgramFiles } else { "$env:USERPROFILE\AppData\Local\Programs" }
Copy-Item $bin (Join-Path $destDir "claude-router.exe") -Force
Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue

Write-Host "Installed claude-router to $destDir\claude-router.exe"
