#!/bin/bash
# Captures a screenshot and rendered DOM dump via headless Chrome.
# Usage: ./snapshot.sh [width] [height] [url]
WIDTH="${1:-1400}"
HEIGHT="${2:-900}"
URL="${3:-http://localhost:8080}"
OUT_IMG="/tmp/hypershift-dashboard.png"
OUT_HTML="/tmp/hypershift-dashboard.html"
CHROME="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
COMMON="--headless=new --disable-gpu --window-size=${WIDTH},${HEIGHT} --virtual-time-budget=15000"

# Screenshot
"$CHROME" $COMMON --screenshot="$OUT_IMG" "$URL" 2>/dev/null
echo "Screenshot: $OUT_IMG ($(wc -c < "$OUT_IMG" | tr -d ' ') bytes)"

# HTML dump (rendered DOM after JS execution)
"$CHROME" $COMMON --dump-dom "$URL" 2>/dev/null > "$OUT_HTML"
echo "HTML dump:  $OUT_HTML ($(wc -c < "$OUT_HTML" | tr -d ' ') bytes)"
