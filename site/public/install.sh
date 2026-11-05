#!/bin/sh
# Install lo0 from its GitHub releases.
#
#   curl -fsSL https://lo0.ai/install.sh | sh
#
# LO0_VERSION    a release tag such as v0.3.0; the newest release by default
# LO0_INSTALL_DIR where to put the binary; ~/.local/bin by default
#
# The binary is checked against the release's checksums.txt before it is
# installed. Nothing else on the machine is changed.
set -eu

# Everything runs from main, called on the last line, so a script cut short
# by a dropped connection runs nothing.
main() {
  repo="https://github.com/daveearley/lo0"
  dir="${LO0_INSTALL_DIR:-$HOME/.local/bin}"

  fail() { printf 'lo0 install: %s\n' "$1" >&2; exit 1; }

  case "$(uname -s)" in
    Darwin) os=darwin ;;
    Linux) os=linux ;;
    *) fail "$(uname -s) is not supported by this script. On Windows run: irm https://lo0.ai/install.ps1 | iex" ;;
  esac

  case "$(uname -m)" in
    x86_64 | amd64) arch=amd64 ;;
    arm64 | aarch64) arch=arm64 ;;
    *) fail "$(uname -m) processors are not supported. lo0 runs on amd64 and arm64." ;;
  esac

  # A Mac running an amd64 shell under Rosetta still wants the arm64 build.
  if [ "$os" = darwin ] && [ "$arch" = amd64 ] && [ "$(sysctl -n sysctl.proc_translated 2>/dev/null || echo 0)" = 1 ]; then
    arch=arm64
  fi

  if [ -n "${LO0_VERSION:-}" ]; then
    base="$repo/releases/download/$LO0_VERSION"
  else
    base="$repo/releases/latest/download"
  fi

  if command -v curl >/dev/null 2>&1; then
    get() { curl -fsSL --proto '=https' --tlsv1.2 -o "$2" "$1"; }
  elif command -v wget >/dev/null 2>&1; then
    get() { wget -q --https-only -O "$2" "$1"; }
  else
    fail "needs curl or wget."
  fi

  if command -v sha256sum >/dev/null 2>&1; then
    sum() { sha256sum "$1" | cut -d ' ' -f 1; }
  elif command -v shasum >/dev/null 2>&1; then
    sum() { shasum -a 256 "$1" | cut -d ' ' -f 1; }
  else
    fail "needs sha256sum or shasum to check the download."
  fi

  name="lo0-$os-$arch"
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT

  printf 'Downloading %s\n' "$name"
  get "$base/$name" "$tmp/lo0" || fail "could not download $base/$name"
  get "$base/checksums.txt" "$tmp/checksums.txt" || fail "could not download $base/checksums.txt"

  want=$(awk -v n="$name" '$2 == n || $2 == "*" n { print $1 }' "$tmp/checksums.txt")
  [ -n "$want" ] || fail "checksums.txt has no entry for $name"
  got=$(sum "$tmp/lo0")
  [ "$want" = "$got" ] || fail "checksum mismatch for $name: expected $want, got $got"

  mkdir -p "$dir"
  chmod 755 "$tmp/lo0"
  mv "$tmp/lo0" "$dir/lo0"

  printf 'Installed %s\n' "$("$dir/lo0" version 2>/dev/null || echo lo0) to $dir/lo0"

  case ":$PATH:" in
    *":$dir:"*) printf '\nRun lo0 to see what your machine can run.\n' ;;
    *)
      printf '\n%s is not on your PATH. Add it with:\n\n' "$dir"
      printf '  export PATH="%s:$PATH"\n\n' "$dir"
      printf 'then run lo0 to see what your machine can run.\n'
      ;;
  esac
}

main "$@"
