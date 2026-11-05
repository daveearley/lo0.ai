#!/bin/sh
# Screenshot `lo0 web` pages in light and dark with headless Chrome.
# usage: web.sh LO0 HOME OUTDIR
set -eu
lo0=$1 home=$2 out=$3
chrome=${CHROME:-"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"}
port=47123
profile=$(mktemp -d)
log=$(mktemp)

HOME=$home "$lo0" web --no-open --port $port >"$log" 2>&1 &
pid=$!
trap 'kill $pid 2>/dev/null; rm -rf "$profile" "$log"' EXIT
sleep 4
token=$(sed -n 's/.*token=\([0-9a-f]*\).*/\1/p' "$log" | head -n 1)

shot() { # page name
  for scheme in dark light; do
    # Chrome's preferredColorScheme: 0 is dark, 1 is light.
    n=0; [ $scheme = light ] && n=1
    "$chrome" --headless=new --disable-gpu --hide-scrollbars --user-data-dir="$profile" \
      --window-size=1440,1000 --force-device-scale-factor=2 --force-prefers-reduced-motion \
      --blink-settings=preferredColorScheme=$n --virtual-time-budget=4000 \
      --screenshot="$out/web-$2-$scheme.png" "http://127.0.0.1:$port/$1?token=$token" >/dev/null 2>&1
  done
}

mkdir -p "$out"
shot "" home
shot models models
shot models/qwen3.6-35b-a3b model
