<#
.SYNOPSIS
  Build moodle-mcp, get your Moodle token and register the server with Claude Code (Windows).

.DESCRIPTION
  The Windows counterpart of setup.sh. Safe to re-run: it replaces an existing registration.

    powershell -ExecutionPolicy Bypass -File .\setup.ps1             interactive
    $env:MOODLE_TOKEN = "…"; powershell -ExecutionPolicy Bypass -File .\setup.ps1
    powershell -ExecutionPolicy Bypass -File .\setup.ps1 -NoSkills

  Your password is sent only to $MoodleUrl/login/token.php (the endpoint the official
  Moodle app uses) and is not stored. The token ends up only in Claude Code's config.
  See docs/STUDENT-SETUP.md for what each step does and for troubleshooting.
#>
[CmdletBinding()]
param(
	[switch]$Skills,
	[switch]$NoSkills,
	[string]$Url
)

Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'
# Windows PowerShell 5.1 on older .NET still offers TLS 1.0 by default.
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

$Repo = 'armenbarseghyan/moodle-mcp'
$MoodleUrl = if ($Url) { $Url } elseif ($env:MOODLE_URL) { $env:MOODLE_URL } else { 'https://moodle.fh-joanneum.at' }
$MoodleUrl = $MoodleUrl.TrimEnd('/')
$ServerName = if ($env:MOODLE_MCP_NAME) { $env:MOODLE_MCP_NAME } else { 'moodle' }
$Root = $PSScriptRoot
# Windows PowerShell 5.1 has no $IsWindows; PowerShell 7 on macOS/Linux runs this too (tests).
$OnWindows = $env:OS -eq 'Windows_NT'
$Bin = Join-Path (Join-Path $Root 'bin') $(if ($OnWindows) { 'moodle-mcp.exe' } else { 'moodle-mcp' })
$SkillMode = if ($Skills) { 'yes' } elseif ($NoSkills) { 'no' } else { 'ask' }

function Step([string]$m) { Write-Host ""; Write-Host "==> $m" }
function Ok([string]$m) { Write-Host "    + $m" -ForegroundColor Green }
function Die([string]$m) {
	Write-Host ""
	Write-Host "    x $m" -ForegroundColor Red
	exit 1
}
function Interactive { -not [Console]::IsInputRedirected }

# Moodle answers errors with HTTP 200 and {"errorcode": …}; this reads a field if present.
function Field($obj, [string]$name) {
	if ($obj -is [string]) { try { $obj = $obj | ConvertFrom-Json } catch { return '' } }
	if ($null -ne $obj -and $obj.PSObject.Properties.Name -contains $name) { return [string]$obj.$name }
	return ''
}

# ---------------------------------------------------------------------------
Step 'Checking prerequisites'
if (-not (Get-Command claude -ErrorAction SilentlyContinue)) {
	Die "Claude Code (the 'claude' CLI) is not on your PATH. Install it from https://claude.com/claude-code and run this script again."
}
Ok 'claude found'

# ---------------------------------------------------------------------------
Step 'Building the server'
New-Item -ItemType Directory -Force -Path (Join-Path $Root 'bin') | Out-Null
if (Get-Command go -ErrorAction SilentlyContinue) {
	$version = 'dev'
	if (Get-Command git -ErrorAction SilentlyContinue) {
		$v = & git -C $Root describe --tags --always --dirty 2>$null
		if ($LASTEXITCODE -eq 0 -and $v) { $version = $v }
	}
	Push-Location $Root
	try { & go build -trimpath -ldflags "-s -w -X main.version=$version" -o $Bin ./cmd/moodle-mcp }
	finally { Pop-Location }
	if ($LASTEXITCODE -ne 0) { Die 'go build failed (the project needs Go 1.26; Go 1.21 or newer downloads it automatically).' }
	Ok "built $Bin ($version)"
} else {
	$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
	$asset = "moodle-mcp-windows-$arch.exe"
	$base = "https://github.com/$Repo/releases/latest/download"
	Write-Host "    Go is not installed; downloading the latest release ($asset)"
	$tmp = Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid())
	New-Item -ItemType Directory -Path $tmp | Out-Null
	try {
		Invoke-WebRequest -UseBasicParsing -Uri "$base/$asset" -OutFile (Join-Path $tmp $asset)
		Invoke-WebRequest -UseBasicParsing -Uri "$base/checksums.txt" -OutFile (Join-Path $tmp 'checksums.txt')
		$line = Get-Content (Join-Path $tmp 'checksums.txt') | Where-Object { $_ -match " $([regex]::Escape($asset))$" } | Select-Object -First 1
		$expected = if ($line) { ($line -split ' ')[0] } else { '' }
		$actual = (Get-FileHash -Algorithm SHA256 (Join-Path $tmp $asset)).Hash.ToLower()
		if (-not $expected -or $expected -ne $actual) { Die "checksum mismatch for $asset - not installing it." }
		Move-Item -Force (Join-Path $tmp $asset) $Bin
	} catch {
		Die "download from $base failed: $($_.Exception.Message)"
	} finally {
		Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
	}
	Ok "downloaded and verified $Bin"
}
& $Bin -version 2>$null | Out-Null
if ($LASTEXITCODE -ne 0) { Die "$Bin does not run." }

# ---------------------------------------------------------------------------
Step 'Getting your Moodle token'
$token = if ($env:MOODLE_TOKEN) { $env:MOODLE_TOKEN } else { '' }
if ($token) {
	Ok 'using MOODLE_TOKEN from the environment'
} else {
	if (-not (Interactive)) { Die 'no console to ask for your login; set $env:MOODLE_TOKEN instead.' }
	Write-Host "    Your FH login is sent once to $MoodleUrl/login/token.php and not stored."
	$username = Read-Host '    FH username'
	$secure = Read-Host '    FH password (hidden)' -AsSecureString
	$password = (New-Object System.Net.NetworkCredential('', $secure)).Password
	if (-not $username -or -not $password) { Die 'username and password are required.' }
	try {
		# A hashtable body is sent form-encoded, so &, =, spaces and umlauts are safe.
		$response = Invoke-RestMethod -Method Post -Uri "$MoodleUrl/login/token.php" `
			-Body @{ username = $username; password = $password; service = 'moodle_mobile_app' }
	} catch {
		Die "could not reach $MoodleUrl ($($_.Exception.Message))."
	} finally {
		Remove-Variable password, secure -ErrorAction SilentlyContinue
	}
	$token = Field $response 'token'
	if (-not $token) {
		$code = Field $response 'errorcode'
		$msg = Field $response 'error'
		switch -Regex ($code) {
			'^invalidlogin$' { Die "Moodle rejected the username or password ($code)." }
			'^(enablewsdescription|servicenotavailable|servicerequireslogin)$' {
				Die "Moodle's mobile web service is not available for your account ($code`: $msg). Ask the FH helpdesk."
			}
			default { Die "Moodle did not return a token ($code`: $msg)." }
		}
	}
	Ok 'token received'
}
# The API key is "token" (32 hex characters). "privatetoken" is something else;
# token+privatetoken pasted together is 64 characters and fails as invalidtoken.
if ($token -cnotmatch '^[0-9a-f]{32}$') {
	Die "that is not a Moodle API token (expected 32 hex characters, got $($token.Length)). Use only the `"token`" value, not `"privatetoken`"."
}

# ---------------------------------------------------------------------------
Step 'Checking the token against Moodle'
try {
	$site = Invoke-RestMethod -Method Post -Uri "$MoodleUrl/webservice/rest/server.php" `
		-Body @{ wstoken = $token; wsfunction = 'core_webservice_get_site_info'; moodlewsrestformat = 'json' }
} catch {
	Die "could not reach $MoodleUrl ($($_.Exception.Message))."
}
$code = Field $site 'errorcode'
if ($code) { Die "Moodle refused the token ($code). Get a new one by running this script again without MOODLE_TOKEN." }
Ok "signed in as $(Field $site 'fullname') ($(Field $site 'username'))"

# ---------------------------------------------------------------------------
Step 'Registering with Claude Code'
& claude mcp get $ServerName *> $null
if ($LASTEXITCODE -eq 0) {
	& claude mcp remove $ServerName -s user *> $null
	Ok "removed the previous '$ServerName' registration"
}
# Claude Code starts MCP servers with a reduced PATH: register the absolute path.
# The arguments go through an array so PowerShell passes "--" on instead of eating it.
$addArgs = @('mcp', 'add', $ServerName, '-s', 'user',
	'-e', "MOODLE_URL=$MoodleUrl", '-e', "MOODLE_TOKEN=$token", '--', $Bin)
& claude @addArgs | Out-Null
Remove-Variable token, addArgs
if ((& claude mcp get $ServerName 2>$null | Out-String) -match 'Connected') {
	Ok "'$ServerName' is registered and connects"
} else {
	Die ("'$ServerName' is registered but does not connect. Run the binary by hand to see why:`n" +
		"      `$env:MOODLE_URL = `"$MoodleUrl`"; `$env:MOODLE_TOKEN = `"<token>`"; & `"$Bin`"`n" +
		"    and read the error it prints (see docs/STUDENT-SETUP.md, Troubleshooting).")
}

# ---------------------------------------------------------------------------
Step 'Claude Code skills'
if ($SkillMode -eq 'ask') {
	$SkillMode = 'yes'
	if (Interactive) {
		$answer = Read-Host '    Install the moodle skills into ~\.claude\skills (recommended)? [Y/n]'
		if ($answer -match '^[nN]') { $SkillMode = 'no' }
	}
}
if ($SkillMode -eq 'yes') {
	$dest = Join-Path $HOME '.claude\skills'
	New-Item -ItemType Directory -Force -Path $dest | Out-Null
	foreach ($dir in Get-ChildItem -Directory (Join-Path $Root 'skills')) {
		$link = Join-Path $dest $dir.Name
		$existing = Get-Item $link -Force -ErrorAction SilentlyContinue
		if ($existing) {
			if (-not ($existing.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
				Write-Host "    ! $link exists and is not a link; left as is"
				continue
			}
			[IO.Directory]::Delete($link) # removes the junction only, not its target
		}
		# Junctions need no administrator rights or developer mode, unlike symlinks.
		New-Item -ItemType Junction -Path $link -Target $dir.FullName | Out-Null
	}
	Ok 'skills linked into ~\.claude\skills (they update with git pull)'
} else {
	Ok 'skipped'
}

Write-Host ""
Write-Host 'Done. Restart Claude Code, then ask: "check my moodle connection" or "what is due this week?"'
