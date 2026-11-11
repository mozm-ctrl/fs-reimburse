# Installs the fsr CLI to %LOCALAPPDATA%\fsr and the skill to ~/.claude/skills/fs-reimburse.
# Usage: powershell -ExecutionPolicy Bypass -File install.ps1 [-SkillsDir DIR] [-BinDir DIR]
param(
  [string]$BinDir = "$env:LOCALAPPDATA\fsr",
  [string]$SkillsDir = "$HOME\.claude\skills"
)
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
  Write-Error "需要 Go 1.26+ 来编译 fsr：https://go.dev/dl/"
}
New-Item -ItemType Directory -Force -Path $BinDir, "$SkillsDir\fs-reimburse" | Out-Null
$env:CGO_ENABLED = "0"
go build -ldflags "-s -w" -o "$BinDir\fsr.exe" .
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Copy-Item SKILL.md "$SkillsDir\fs-reimburse\SKILL.md" -Force
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (($userPath -split ";") -notcontains $BinDir) {
  [Environment]::SetEnvironmentVariable("Path", "$userPath;$BinDir", "User")
  Write-Host "已把 $BinDir 加到用户 PATH，重新打开终端后生效"
}
Write-Host "已安装 fsr 到 $BinDir\fsr.exe，skill 到 $SkillsDir\fs-reimburse"
Write-Host "下一步：运行 fsr login 登录报销网站"
