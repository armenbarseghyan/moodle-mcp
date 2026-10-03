#!/usr/bin/env bash
# dev/setup_test.sh — end-to-end test of ./setup.sh without touching the real
# Moodle or the real Claude Code config: the fake Moodle (dev/fakemoodle)
# serves login/token.php and the web service, a stub `claude` records calls,
# and HOME points to a temporary directory.
#
#   make setup-test

set -uo pipefail # no -e: a failing check must be reported, not abort the run

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# The runs use a temporary HOME; keep Go's caches so setup.sh's build is fast
# and offline.
GOMODCACHE="$(go env GOMODCACHE)"
GOCACHE="$(go env GOCACHE)"
export GOMODCACHE GOCACHE
WORK="$(mktemp -d)"
FAKE_PID=""
cleanup() {
	[ -n "$FAKE_PID" ] && kill "$FAKE_PID" 2>/dev/null || true
	chmod -R u+w "$WORK" 2>/dev/null || true
	rm -rf "$WORK"
}
trap cleanup EXIT

pass=0
fail=0
check() { # check NAME CONDITION-EXIT-CODE
	if [ "$2" -eq 0 ]; then
		printf '  ✓ %s\n' "$1"
		pass=$((pass + 1))
	else
		printf '  ✗ %s\n' "$1"
		fail=$((fail + 1))
	fi
}

# --- fake Moodle --------------------------------------------------------------
(cd "$ROOT" && go build -o "$WORK/fakemoodle" ./dev/fakemoodle)
"$WORK/fakemoodle" -url-file "$WORK/url" >/dev/null 2>&1 &
FAKE_PID=$!
for _ in $(seq 50); do [ -s "$WORK/url" ] && break; sleep 0.1; done
URL="$(cat "$WORK/url")"
TOKEN="$("$WORK/fakemoodle" -token)"
CREDS="$("$WORK/fakemoodle" -creds)"
USERNAME="$(printf '%s\n' "$CREDS" | sed -n 1p)"
PASSWORD="$(printf '%s\n' "$CREDS" | sed -n 2p)"

# --- stub claude ----------------------------------------------------------------
mkdir -p "$WORK/stub"
cat >"$WORK/stub/claude" <<'STUB'
#!/usr/bin/env bash
# Records every call; "mcp get" reports Connected once "mcp add" ran, unless
# STUB_NEVER_CONNECTS is set.
log="$STUB_DIR/calls.log"
printf '%s\n' "$*" >>"$log"
case "$1 $2" in
"mcp add") touch "$STUB_DIR/registered" ;;
"mcp remove") rm -f "$STUB_DIR/registered" ;;
"mcp get")
	[ -f "$STUB_DIR/registered" ] || exit 1
	if [ -n "${STUB_NEVER_CONNECTS:-}" ]; then echo "Status: ✘ Failed to connect"; else echo "Status: ✔ Connected"; fi
	;;
esac
STUB
chmod +x "$WORK/stub/claude"

# run_setup NAME [ENV...] -- [ARGS...]: runs setup.sh in a fresh HOME.
run_setup() {
	local name="$1"
	shift
	local dir="$WORK/$name"
	mkdir -p "$dir/home"
	env HOME="$dir/home" PATH="$WORK/stub:$PATH" STUB_DIR="$dir" MOODLE_URL="$URL" "$@" \
		"$ROOT/setup.sh" --no-skills >"$dir/out" 2>&1
}

# Interactive runs need a terminal: drive setup.sh through a pseudo-terminal
# that answers the username and password prompts.
# run_interactive NAME USERNAME PASSWORD [SETUP-COMMAND...] (default ./setup.sh)
run_interactive() {
	local name="$1" username="$2" password="$3"
	shift 3
	[ $# -gt 0 ] || set -- "$ROOT/setup.sh" --no-skills
	local dir="$WORK/$name"
	mkdir -p "$dir/home"
	env -u MOODLE_TOKEN HOME="$dir/home" PATH="$WORK/stub:$PATH" STUB_DIR="$dir" MOODLE_URL="$URL" \
		python3 "$ROOT/dev/pty_run.py" "FH username: =$username" "FH password (hidden): =$password" -- \
		"$@" >"$dir/out" 2>&1
}

echo "setup.sh end-to-end (fake Moodle at $URL)"

# 1. Token from the environment: build, verify, register by absolute path.
rc=0
run_setup token-env MOODLE_TOKEN="$TOKEN" || rc=$?
check "succeeds with MOODLE_TOKEN" "$rc"
grep -q "signed in as Max Student (s00000)" "$WORK/token-env/out"
check "verifies the token against the site" $?
grep -q "^mcp add moodle -s user -e MOODLE_URL=$URL -e MOODLE_TOKEN=$TOKEN -- $ROOT/bin/moodle-mcp\$" "$WORK/token-env/calls.log"
check "registers the absolute binary path with both variables" $?
if grep -q "$TOKEN" "$WORK/token-env/out"; then rc=1; else rc=0; fi
check "never prints the token" "$rc"

# 2. Interactive login through login/token.php with a password that needs encoding.
rc=0
run_interactive login-ok "$USERNAME" "$PASSWORD" || rc=$?
check "interactive login succeeds (password with space, &, =, ü)" "$rc"
grep -q "token received" "$WORK/login-ok/out"
check "takes token, not privatetoken" $?
grep -q "MOODLE_TOKEN=$TOKEN " "$WORK/login-ok/calls.log"
check "registers exactly the 32-character token" $?
if grep -q -- "$PASSWORD" "$WORK/login-ok/out" "$WORK/login-ok/calls.log"; then rc=1; else rc=0; fi
check "never echoes or passes on the password" "$rc"

# 3. Wrong password: clear message, nothing registered.
rc=0
run_interactive login-bad "$USERNAME" wrong || rc=$?
[ "$rc" -ne 0 ] && grep -q "rejected the username or password (invalidlogin)" "$WORK/login-bad/out"
check "wrong password fails with invalidlogin" $?
[ ! -f "$WORK/login-bad/calls.log" ] || ! grep -q "mcp add" "$WORK/login-bad/calls.log"
check "nothing registered after a failed login" $?

# 4. token+privatetoken pasted together (64 characters).
rc=0
run_setup pasted-both MOODLE_TOKEN="$TOKEN$("$WORK/fakemoodle" -token | cut -c1-32)" || rc=$?
[ "$rc" -ne 0 ] && grep -q 'expected 32 hex characters, got 64' "$WORK/pasted-both/out"
check "rejects a 64-character token+privatetoken paste" $?

# 5. A revoked token is caught before registering.
rc=0
run_setup revoked MOODLE_TOKEN="0123456789abcdef0123456789abcdef" || rc=$?
[ "$rc" -ne 0 ] && grep -q "refused the token (invalidtoken)" "$WORK/revoked/out"
check "rejects a token Moodle does not accept" $?

# 6. Registered but not connecting: points at running the binary by hand.
rc=0
run_setup no-connect MOODLE_TOKEN="$TOKEN" STUB_NEVER_CONNECTS=1 || rc=$?
[ "$rc" -ne 0 ] && grep -q "Run the binary by hand" "$WORK/no-connect/out"
check "explains how to debug a server that does not connect" $?

# 7. setup.ps1's interactive login, when PowerShell 7 is available (PWSH=path or
# pwsh on PATH). Its other paths are covered by dev/setup_test.ps1.
PWSH="${PWSH:-$(command -v pwsh || true)}"
if [ -n "$PWSH" ]; then
	rc=0
	run_interactive ps-login-ok "$USERNAME" "$PASSWORD" "$PWSH" -NoProfile -File "$ROOT/setup.ps1" -NoSkills || rc=$?
	[ "$rc" -eq 0 ] && grep -q "MOODLE_TOKEN=$TOKEN " "$WORK/ps-login-ok/calls.log"
	check "setup.ps1: interactive login registers the 32-character token" $?
	if grep -q -- "$PASSWORD" "$WORK/ps-login-ok/out" "$WORK/ps-login-ok/calls.log"; then rc=1; else rc=0; fi
	check "setup.ps1: never echoes or passes on the password" "$rc"
	rc=0
	run_interactive ps-login-bad "$USERNAME" wrong "$PWSH" -NoProfile -File "$ROOT/setup.ps1" -NoSkills || rc=$?
	[ "$rc" -ne 0 ] && grep -q "rejected the username or password (invalidlogin)" "$WORK/ps-login-bad/out"
	check "setup.ps1: wrong password fails with invalidlogin" $?
else
	echo "  - setup.ps1 interactive checks skipped (no pwsh; set PWSH=/path/to/pwsh)"
fi

printf '\n%d passed, %d failed\n' "$pass" "$fail"
if [ "$fail" -ne 0 ]; then
	for out in "$WORK"/*/out; do printf '\n--- %s\n' "$out"; cat "$out"; done
	exit 1
fi
