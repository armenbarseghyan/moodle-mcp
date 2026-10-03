# dev/setup_test.ps1 — end-to-end test of setup.ps1 on Windows, the counterpart of
# dev/setup_test.sh: the fake Moodle (dev/fakemoodle) serves the web service, a stub
# claude.cmd records calls, nothing touches the real Moodle or Claude Code config.
# The interactive login is covered by dev/setup_test.sh (it needs a console).
# Also runs under PowerShell 7 on macOS/Linux (with a shell stub instead of claude.cmd).
#
#   pwsh -File dev/setup_test.ps1                 PowerShell 7
#   powershell -File dev/setup_test.ps1           Windows PowerShell 5.1

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
$Shell = (Get-Process -Id $PID).Path # run setup.ps1 with the same PowerShell
# The runs use a temporary home; keep Go's caches so the build stays fast and offline.
$env:GOMODCACHE = (& go env GOMODCACHE)
$env:GOCACHE = (& go env GOCACHE)
$Work = Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid())
New-Item -ItemType Directory -Path $Work | Out-Null
$fake = $null
$script:pass = 0
$script:fail = 0

function Check([string]$name, [bool]$ok) {
	if ($ok) { Write-Host "  + $name"; $script:pass++ } else { Write-Host "  x $name"; $script:fail++ }
}

try {
	# --- fake Moodle ------------------------------------------------------------
	Push-Location $Root
	try { & go build -o (Join-Path $Work 'fakemoodle.exe') ./dev/fakemoodle } finally { Pop-Location }
	if ($LASTEXITCODE -ne 0) { throw 'building fakemoodle failed' }
	$urlFile = Join-Path $Work 'url'
	$onWindows = $env:OS -eq 'Windows_NT'
	$hidden = if ($onWindows) { @{ WindowStyle = 'Hidden' } } else { @{} } # -WindowStyle is Windows-only
	$fake = Start-Process -PassThru @hidden -RedirectStandardError (Join-Path $Work 'fakemoodle.log') -RedirectStandardOutput (Join-Path $Work 'fakemoodle.out') `
		-FilePath (Join-Path $Work 'fakemoodle.exe') -ArgumentList '-url-file', $urlFile
	for ($i = 0; $i -lt 100 -and -not (Test-Path $urlFile); $i++) { Start-Sleep -Milliseconds 100 }
	$Url = (Get-Content $urlFile -Raw).Trim()
	$Token = (& (Join-Path $Work 'fakemoodle.exe') -token).Trim()

	# --- stub claude ------------------------------------------------------------
	$stub = Join-Path $Work 'stub'
	New-Item -ItemType Directory -Path $stub | Out-Null
	if (-not $onWindows) { # PowerShell 7 on macOS/Linux: a shell stub, same behaviour
		Set-Content -Path (Join-Path $stub 'claude') -Value @'
#!/bin/sh
printf '%s\n' "$*" >>"$STUB_DIR/calls.log"
case "$1 $2" in
"mcp add") touch "$STUB_DIR/registered" ;;
"mcp remove") rm -f "$STUB_DIR/registered" ;;
"mcp get")
	[ -f "$STUB_DIR/registered" ] || exit 1
	if [ -n "${STUB_NEVER_CONNECTS:-}" ]; then echo "Status: Failed to connect"; else echo "Status: Connected"; fi ;;
esac
'@
		& chmod +x (Join-Path $stub 'claude')
	} else {
	Set-Content -Encoding ascii -Path (Join-Path $stub 'claude.cmd') -Value @'
@echo off
echo %*>>"%STUB_DIR%\calls.log"
if "%1 %2"=="mcp add" type nul > "%STUB_DIR%\registered"
if "%1 %2"=="mcp remove" del "%STUB_DIR%\registered" 2>nul
if not "%1 %2"=="mcp get" exit /b 0
if not exist "%STUB_DIR%\registered" exit /b 1
if defined STUB_NEVER_CONNECTS (echo Status: Failed to connect) else (echo Status: Connected)
exit /b 0
'@
	}

	# --- stub codex: "mcp add" stores the arguments, "mcp get" prints the command ---
	$stubCodex = Join-Path $Work 'stub-codex'
	New-Item -ItemType Directory -Path $stubCodex | Out-Null
	if (-not $onWindows) {
		Set-Content -Path (Join-Path $stubCodex 'codex') -Value @'
#!/bin/sh
printf '%s\n' "$*" >>"$STUB_DIR/codex.log"
case "$1 $2" in
"mcp add") for a in "$@"; do last="$a"; done; printf '%s' "$last" >"$STUB_DIR/codex-command" ;;
"mcp get")
	[ -f "$STUB_DIR/codex-command" ] || exit 1
	printf '%s\n  transport: stdio\n  command: %s\n' "$3" "$(cat "$STUB_DIR/codex-command")" ;;
esac
'@
		& chmod +x (Join-Path $stubCodex 'codex')
	} else {
		# cmd splits %1..%9 at "=" too, so the command is cut from %* after "-- ".
		Set-Content -Encoding ascii -Path (Join-Path $stubCodex 'codex.cmd') -Value @'
@echo off
echo %*>>"%STUB_DIR%\codex.log"
set "all=%*"
if "%1 %2"=="mcp add" (
  setlocal enabledelayedexpansion
  set "cmd=!all:*-- =!"
  set "cmd=!cmd:"=!"
  >"%STUB_DIR%\codex-command" echo !cmd!
  endlocal
  exit /b 0
)
if not "%1 %2"=="mcp get" exit /b 0
if not exist "%STUB_DIR%\codex-command" exit /b 1
set /p cmd=<"%STUB_DIR%\codex-command"
echo %3
echo   transport: stdio
echo   command: %cmd%
exit /b 0
'@
	}

	# Run-Setup NAME ENV-VARS [SETUP-ARGS] [STUB-DIRS]; default -Claude -NoSkills
	# with only the claude stub on PATH.
	function Run-Setup([string]$name, [hashtable]$envVars, [string[]]$setupArgs = @('-Claude', '-NoSkills'), [string[]]$stubs = @($stub)) {
		# Windows PowerShell 5.1 turns the child's stderr into error records.
		$ErrorActionPreference = 'Continue'
		$dir = Join-Path $Work $name
		New-Item -ItemType Directory -Path $dir | Out-Null
		New-Item -ItemType Directory -Path (Join-Path $dir 'home') | Out-Null
		$saved = @{}
		$sep = [IO.Path]::PathSeparator
		$vars = @{ PATH = (($stubs + $env:PATH) -join $sep); STUB_DIR = $dir; MOODLE_URL = $Url; MOODLE_TOKEN = $null; STUB_NEVER_CONNECTS = $null
			HOME = (Join-Path $dir 'home'); USERPROFILE = (Join-Path $dir 'home') }
		foreach ($k in $envVars.Keys) { $vars[$k] = $envVars[$k] }
		foreach ($k in $vars.Keys) {
			$saved[$k] = [Environment]::GetEnvironmentVariable($k)
			[Environment]::SetEnvironmentVariable($k, $vars[$k])
		}
		try {
			$out = & $Shell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $Root 'setup.ps1') @setupArgs 2>&1 | Out-String
			$code = $LASTEXITCODE
		} finally {
			foreach ($k in $saved.Keys) { [Environment]::SetEnvironmentVariable($k, $saved[$k]) }
		}
		Set-Content -Path (Join-Path $dir 'out') -Value $out
		$calls = if (Test-Path (Join-Path $dir 'calls.log')) { Get-Content -Raw (Join-Path $dir 'calls.log') } else { '' }
		$codexCalls = if (Test-Path (Join-Path $dir 'codex.log')) { Get-Content -Raw (Join-Path $dir 'codex.log') } else { '' }
		return @{ Code = $code; Out = $out; Calls = $calls; Codex = $codexCalls; Home = (Join-Path $dir 'home') }
	}

	Write-Host "setup.ps1 end-to-end with $Shell (fake Moodle at $Url)"
	$bin = Join-Path (Join-Path $Root 'bin') $(if ($onWindows) { 'moodle-mcp.exe' } else { 'moodle-mcp' })

	# 1. Token from the environment: build, verify, register by absolute path.
	$r = Run-Setup 'token-env' @{ MOODLE_TOKEN = $Token }
	Check 'succeeds with MOODLE_TOKEN' ($r.Code -eq 0)
	Check 'verifies the token against the site' ($r.Out -match 'signed in as Max Student \(s00000\)')
	Check 'registers the absolute binary path with both variables' `
		($r.Calls.Contains("mcp add moodle -s user -e MOODLE_URL=$Url -e MOODLE_TOKEN=$Token -- $bin"))
	Check 'never prints the token' (-not $r.Out.Contains($Token))

	# 2. token+privatetoken pasted together (64 characters).
	$r = Run-Setup 'pasted-both' @{ MOODLE_TOKEN = $Token + $Token }
	Check 'rejects a 64-character token+privatetoken paste' (($r.Code -ne 0) -and ($r.Out -match 'expected 32 hex characters, got 64'))
	Check 'nothing registered after a rejected token' (-not $r.Calls.Contains('mcp add'))

	# 3. A revoked token is caught before registering.
	$r = Run-Setup 'revoked' @{ MOODLE_TOKEN = '0123456789abcdef0123456789abcdef' }
	Check 'rejects a token Moodle does not accept' (($r.Code -ne 0) -and ($r.Out -match 'refused the token \(invalidtoken\)'))

	# 4. Registered but not connecting: points at running the binary by hand.
	$r = Run-Setup 'no-connect' @{ MOODLE_TOKEN = $Token; STUB_NEVER_CONNECTS = '1' }
	Check 'explains how to debug a server that does not connect' (($r.Code -ne 0) -and ($r.Out -match 'Run the binary by hand'))

	# 5. Codex only: --env and the absolute path, Claude Code untouched.
	$r = Run-Setup 'codex-only' @{ MOODLE_TOKEN = $Token } @('-Codex', '-NoSkills') @($stub, $stubCodex)
	Check '-Codex succeeds' ($r.Code -eq 0)
	Check '-Codex registers the absolute binary path with both variables' `
		($r.Codex.Contains("mcp add moodle --env MOODLE_URL=$Url --env MOODLE_TOKEN=$Token -- $bin"))
	Check '-Codex leaves Claude Code alone' (-not $r.Calls)

	# 6. Default: every installed client, skills linked for each.
	$r = Run-Setup 'both' @{ MOODLE_TOKEN = $Token } @('-Skills') @($stub, $stubCodex)
	Check 'default registers with every installed client' (($r.Code -eq 0) -and $r.Calls.Contains('mcp add moodle') -and $r.Codex.Contains('mcp add moodle'))
	Check 'skills linked for Claude Code and Codex' `
		((Test-Path (Join-Path $r.Home '.claude/skills/moodle-setup/SKILL.md')) -and (Test-Path (Join-Path $r.Home '.agents/skills/moodle-briefing/SKILL.md')))
	Check 'tells which clients to restart' ($r.Out -match 'Restart Claude Code and Codex')

	Write-Host ""
	Write-Host "$script:pass passed, $script:fail failed"
	if ($script:fail -ne 0) {
		Get-ChildItem $Work -Directory | ForEach-Object {
			$o = Join-Path $_.FullName 'out'
			if (Test-Path $o) { Write-Host "--- $o"; Get-Content $o }
		}
	}
} finally {
	if ($fake) { Stop-Process -Id $fake.Id -ErrorAction SilentlyContinue }
	Remove-Item -Recurse -Force $Work -ErrorAction SilentlyContinue
}
# Explicit: the exit code would otherwise be that of the last setup.ps1 run.
if ($script:fail -ne 0) { exit 1 }
exit 0
