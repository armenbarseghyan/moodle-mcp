#!/usr/bin/env bash
# setup.sh — build moodle-mcp, get your Moodle token and register the server
# with Claude Code. Safe to re-run: it replaces an existing registration.
#
#   ./setup.sh                 interactive (asks for your FH login once)
#   MOODLE_TOKEN=… ./setup.sh  use a token you already have
#   ./setup.sh --no-skills     don't install the Claude Code skills
#
# Your password is sent only to $MOODLE_URL/login/token.php (the endpoint the
# official Moodle app uses), through stdin so it never shows up in the process
# list, and is not stored. The token ends up only in Claude Code's config.
# See docs/STUDENT-SETUP.md for what each step does and for troubleshooting.

set -euo pipefail

REPO="armenbarseghyan/moodle-mcp"
MOODLE_URL="${MOODLE_URL:-https://moodle.fh-joanneum.at}"
MOODLE_URL="${MOODLE_URL%/}"
SERVER_NAME="${MOODLE_MCP_NAME:-moodle}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN="$ROOT/bin/moodle-mcp"
SKILLS="ask"

usage() {
	sed -n '2,13p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
}

while [ $# -gt 0 ]; do
	case "$1" in
	--skills) SKILLS="yes" ;;
	--no-skills) SKILLS="no" ;;
	--url)
		[ $# -ge 2 ] || { usage >&2; exit 2; }
		MOODLE_URL="${2%/}"
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		printf 'unknown option: %s\n\n' "$1" >&2
		usage >&2
		exit 2
		;;
	esac
	shift
done

step() { printf '\n==> %s\n' "$*"; }
ok() { printf '    ✓ %s\n' "$*"; }
die() {
	printf '\n    ✗ %s\n' "$*" >&2
	exit 1
}
interactive() { [ -t 0 ]; }

# json_field NAME — the string value of "NAME" in the JSON on stdin, compact or
# pretty-printed (values here contain no quotes).
# Anchored on the opening quote, so "token" never matches "privatetoken".
json_field() {
	sed -n "s/.*\"$1\"[[:space:]]*:[[:space:]]*\"\\([^\"]*\\)\".*/\\1/p" | head -n 1
}

# ---------------------------------------------------------------------------
step "Checking prerequisites"
command -v curl >/dev/null || die "curl is required."
command -v claude >/dev/null || die "Claude Code (the 'claude' CLI) is not on your PATH. Install it from https://claude.com/claude-code and run this script again."
ok "curl and claude found"

# ---------------------------------------------------------------------------
step "Building the server"
mkdir -p "$ROOT/bin"
if command -v go >/dev/null; then
	version="$(git -C "$ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)"
	(cd "$ROOT" && go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$BIN" ./cmd/moodle-mcp) ||
		die "go build failed (the project needs Go 1.26; Go 1.21 or newer downloads it automatically)."
	ok "built $BIN ($version)"
else
	os="$(uname -s | tr '[:upper:]' '[:lower:]')"
	arch="$(uname -m)"
	case "$arch" in
	x86_64 | amd64) arch="amd64" ;;
	arm64 | aarch64) arch="arm64" ;;
	*) die "no prebuilt binary for $arch — install Go from https://go.dev/dl and run this script again." ;;
	esac
	case "$os" in darwin | linux) ;; *) die "no prebuilt binary for $os — install Go and run this script again." ;; esac
	asset="moodle-mcp-$os-$arch"
	base="https://github.com/$REPO/releases/latest/download"
	printf '    Go is not installed; downloading the latest release (%s)\n' "$asset"
	tmp="$(mktemp -d)"
	trap 'rm -rf "$tmp"' EXIT
	curl -fsSL "$base/$asset" -o "$tmp/$asset" || die "download of $base/$asset failed."
	curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt" || die "download of checksums.txt failed."
	expected="$(grep " $asset\$" "$tmp/checksums.txt" | cut -d' ' -f1)"
	if command -v shasum >/dev/null; then
		actual="$(shasum -a 256 "$tmp/$asset" | cut -d' ' -f1)"
	else
		actual="$(sha256sum "$tmp/$asset" | cut -d' ' -f1)"
	fi
	[ -n "$expected" ] && [ "$expected" = "$actual" ] || die "checksum mismatch for $asset — not installing it."
	mv "$tmp/$asset" "$BIN"
	chmod +x "$BIN"
	ok "downloaded and verified $BIN"
fi
"$BIN" -version >/dev/null 2>&1 || die "$BIN does not run."

# ---------------------------------------------------------------------------
step "Getting your Moodle token"
token="${MOODLE_TOKEN:-}"
if [ -n "$token" ]; then
	ok "using MOODLE_TOKEN from the environment"
else
	interactive || die "no terminal to ask for your login; run with MOODLE_TOKEN=<token> instead."
	printf '    Your FH login is sent once to %s/login/token.php and not stored.\n' "$MOODLE_URL"
	printf '    Type it (don'"'"'t paste several lines at once).\n'
	read -r -p "    FH username: " username
	read -r -s -p "    FH password (hidden): " password
	printf '\n'
	[ -n "$username" ] && [ -n "$password" ] || die "username and password are required."
	response="$(printf '%s' "$password" | curl -sS -X POST "$MOODLE_URL/login/token.php" \
		--data-urlencode "username=$username" \
		--data-urlencode "password@-" \
		--data-urlencode "service=moodle_mobile_app")" || die "could not reach $MOODLE_URL."
	unset password
	token="$(printf '%s' "$response" | json_field token)"
	if [ -z "$token" ]; then
		code="$(printf '%s' "$response" | json_field errorcode)"
		msg="$(printf '%s' "$response" | json_field error)"
		case "$code" in
		invalidlogin) die "Moodle rejected the username or password ($code)." ;;
		enablewsdescription | servicenotavailable | servicerequireslogin)
			die "Moodle's mobile web service is not available for your account ($code: $msg). Ask the FH helpdesk." ;;
		*) die "Moodle did not return a token (${code:-no errorcode}: ${msg:-unexpected answer})." ;;
		esac
	fi
	ok "token received"
fi
# The API key is "token" (32 hex characters). "privatetoken" is something else;
# token+privatetoken pasted together is 64 characters and fails as invalidtoken.
printf '%s' "$token" | grep -Eq '^[0-9a-f]{32}$' ||
	die "that is not a Moodle API token (expected 32 hex characters, got ${#token}). Use only the \"token\" value, not \"privatetoken\"."

# ---------------------------------------------------------------------------
step "Checking the token against Moodle"
site="$(printf '%s' "$token" | curl -sS -X POST "$MOODLE_URL/webservice/rest/server.php" \
	--data-urlencode "wstoken@-" \
	--data-urlencode "wsfunction=core_webservice_get_site_info" \
	--data-urlencode "moodlewsrestformat=json")" || die "could not reach $MOODLE_URL."
code="$(printf '%s' "$site" | json_field errorcode)"
[ -z "$code" ] || die "Moodle refused the token ($code). Get a new one by running this script again without MOODLE_TOKEN."
fullname="$(printf '%s' "$site" | json_field fullname)"
username_ws="$(printf '%s' "$site" | json_field username)"
ok "signed in as ${fullname:-?} (${username_ws:-?})"

# ---------------------------------------------------------------------------
step "Registering with Claude Code"
if claude mcp get "$SERVER_NAME" >/dev/null 2>&1; then
	claude mcp remove "$SERVER_NAME" -s user >/dev/null 2>&1 || true
	ok "removed the previous '$SERVER_NAME' registration"
fi
# Claude Code starts MCP servers with a reduced PATH: register the absolute path.
claude mcp add "$SERVER_NAME" -s user \
	-e "MOODLE_URL=$MOODLE_URL" \
	-e "MOODLE_TOKEN=$token" \
	-- "$BIN" >/dev/null
unset token
if claude mcp get "$SERVER_NAME" 2>/dev/null | grep -q "Connected"; then
	ok "'$SERVER_NAME' is registered and connects"
else
	die "'$SERVER_NAME' is registered but does not connect. Run the binary by hand to see why:
      MOODLE_URL=$MOODLE_URL MOODLE_TOKEN=<token> $BIN
    and read the error on stderr (see docs/STUDENT-SETUP.md, Troubleshooting)."
fi

# ---------------------------------------------------------------------------
step "Claude Code skills"
if [ "$SKILLS" = "ask" ]; then
	if interactive; then
		read -r -p "    Install the moodle skills into ~/.claude/skills (recommended)? [Y/n] " answer
		case "$answer" in [nN]*) SKILLS="no" ;; *) SKILLS="yes" ;; esac
	else
		SKILLS="yes"
	fi
fi
if [ "$SKILLS" = "yes" ]; then
	mkdir -p "$HOME/.claude/skills"
	for dir in "$ROOT"/skills/*/; do
		name="$(basename "$dir")"
		ln -sfn "${dir%/}" "$HOME/.claude/skills/$name"
	done
	ok "skills linked into ~/.claude/skills (they update with git pull)"
else
	ok "skipped"
fi

printf '\nDone. Restart Claude Code, then ask: "check my moodle connection" or "what is due this week?"\n'
