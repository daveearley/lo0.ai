#!/bin/sh
# Tests site/public/install.sh against a release built on this machine.
#
#   site/test/install.sh DIST VERSION
#
# DIST holds lo0-<os>-<arch> binaries and checksums.txt, as make dist writes
# them and a GitHub release publishes them; VERSION is what the binaries
# report. The installer runs unchanged, piped to sh as the quick start says,
# with a curl on PATH that serves DIST in place of GitHub. Everything it
# installs goes into a temporary directory.
set -eu

[ $# -eq 2 ] || { echo "usage: $0 DIST VERSION" >&2; exit 2; }
dist=$(cd "$1" && pwd)
version=$2
installer=$(cd "$(dirname "$0")/../public" && pwd)/install.sh
repo="https://github.com/daveearley/lo0/releases"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
failures=0

fail() { printf 'FAIL %s\n' "$1" >&2; failures=$((failures + 1)); }
pass() { printf 'ok   %s\n' "$1"; }

# A curl that answers from $SERVE and logs each URL it was asked for.
mkdir -p "$work/shim"
cat >"$work/shim/curl" <<'EOF'
#!/bin/sh
out= url=
while [ $# -gt 0 ]; do
  case $1 in
    -o) out=$2; shift 2 ;;
    --proto) shift 2 ;;
    -*) shift ;;
    *) url=$1; shift ;;
  esac
done
echo "$url" >>"$CURL_LOG"
case $url in
  https://github.com/daveearley/lo0/releases/*) ;;
  *) echo "curl shim: unexpected URL $url" >&2; exit 22 ;;
esac
[ -f "$SERVE/${url##*/}" ] || { echo "curl: (22) 404 for $url" >&2; exit 22; }
cp "$SERVE/${url##*/}" "$out"
EOF
chmod 755 "$work/shim/curl"

# install runs the installer the way the quick start does, as a fresh user
# whose home is $1, with extra environment after it.
install() {
  home=$1
  shift
  mkdir -p "$home"
  : >"$work/urls"
  env -i HOME="$home" PATH="$work/shim:/usr/bin:/bin:/usr/sbin:/sbin" \
    CURL_LOG="$work/urls" SERVE="${SERVE:-$dist}" "$@" \
    sh <"$installer" >"$work/out" 2>&1
}

case "$(uname -s)" in Darwin) os=darwin ;; *) os=linux ;; esac
case "$(uname -m)" in x86_64 | amd64) arch=amd64 ;; *) arch=arm64 ;; esac
name="lo0-$os-$arch"

# 1. The default: newest release into ~/.local/bin, which is not on PATH.
home="$work/fresh"
if install "$home"; then
  bin="$home/.local/bin/lo0"
  if [ -x "$bin" ] && [ "$("$bin" version)" = "lo0 $version" ]; then
    pass "installs $name to ~/.local/bin"
  else
    fail "default install: $bin missing or not lo0 $version"; cat "$work/out" >&2
  fi
  grep -qx "$repo/latest/download/$name" "$work/urls" || fail "default install did not fetch the latest release"
  grep -q "Installed lo0 $version" "$work/out" || fail "default install did not say what it installed"
  grep -q 'export PATH=' "$work/out" || fail "default install did not explain how to add ~/.local/bin to PATH"
else
  fail "default install exited non-zero"; cat "$work/out" >&2
fi

# 2. The quick start's next step: run lo0 on a machine with no history.
if [ -x "$home/.local/bin/lo0" ]; then
  if HOME="$home" LO0_OFFLINE=1 "$home/.local/bin/lo0" match >"$work/match" 2>&1 && grep -q Verdict "$work/match"; then
    pass "lo0 match runs straight after install"
  else
    fail "lo0 match after install"; cat "$work/match" >&2
  fi
fi

# 3. A pinned version into a directory already on PATH.
home="$work/pinned"
if install "$home" LO0_VERSION=v9.9.9 LO0_INSTALL_DIR="$home/bin" PATH="$home/bin:$work/shim:/usr/bin:/bin"; then
  grep -qx "$repo/download/v9.9.9/$name" "$work/urls" || fail "LO0_VERSION did not pick the release: $(cat "$work/urls")"
  [ -x "$home/bin/lo0" ] || fail "LO0_INSTALL_DIR was not used"
  grep -q 'Run lo0' "$work/out" || fail "an install onto PATH did not say to run lo0"
  pass "honours LO0_VERSION and LO0_INSTALL_DIR"
else
  fail "pinned install exited non-zero"; cat "$work/out" >&2
fi

# 4. A binary that does not match checksums.txt is never installed.
tampered="$work/tampered-dist"
mkdir -p "$tampered"
cp "$dist/checksums.txt" "$tampered/"
printf 'not lo0\n' >"$tampered/$name"
home="$work/tampered"
if SERVE="$tampered" install "$home"; then
  fail "a tampered binary was accepted"
elif [ -e "$home/.local/bin/lo0" ]; then
  fail "a tampered binary was installed"
elif grep -q 'checksum mismatch' "$work/out"; then
  pass "refuses a binary that does not match checksums.txt"
else
  fail "tampered install failed without saying why"; cat "$work/out" >&2
fi

# 5. A release without this platform in checksums.txt is refused.
missing="$work/missing-dist"
mkdir -p "$missing"
cp "$dist/$name" "$missing/"
grep -v " \*\{0,1\}$name\$" "$dist/checksums.txt" >"$missing/checksums.txt" || true
home="$work/missing"
if SERVE="$missing" install "$home"; then
  fail "a binary without a checksum was accepted"
elif grep -q 'no entry' "$work/out" && [ ! -e "$home/.local/bin/lo0" ]; then
  pass "refuses a binary checksums.txt does not list"
else
  fail "missing-checksum install failed without saying why"; cat "$work/out" >&2
fi

# 6. A script cut short in transit, here just before the closing main call,
# runs nothing.
home="$work/truncated"
mkdir -p "$home"
if sed '$d' "$installer" | env -i HOME="$home" PATH="$work/shim:/usr/bin:/bin" \
  CURL_LOG="$work/urls" SERVE="$dist" sh >"$work/out" 2>&1 &&
  [ ! -e "$home/.local/bin" ] && [ ! -s "$work/out" ]; then
  pass "a truncated script runs nothing"
else
  fail "a truncated script ran"; cat "$work/out" >&2
fi

[ "$failures" -eq 0 ] || { printf '%d installer checks failed\n' "$failures" >&2; exit 1; }
