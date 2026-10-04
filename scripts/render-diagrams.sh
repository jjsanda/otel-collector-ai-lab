#!/usr/bin/env bash
# Render every Mermaid diagram in docs/diagrams/*.mmd to SVG and PNG using
# mermaid-cli (mmdc). Requires Node/npx; downloads a headless Chromium on first
# run. Output lands in docs/diagrams/rendered/.
set -euo pipefail
cd "$(dirname "$0")/.."

SRC="docs/diagrams"
OUT="docs/diagrams/rendered"
PCONF="scripts/puppeteer-config.json"
mkdir -p "$OUT"

for f in "$SRC"/*.mmd; do
  name="$(basename "$f" .mmd)"
  echo "rendering $name ..."
  npx -y @mermaid-js/mermaid-cli -i "$f" -o "$OUT/$name.svg" -b transparent -p "$PCONF"
  npx -y @mermaid-js/mermaid-cli -i "$f" -o "$OUT/$name.png" -b white -s 2 -p "$PCONF"
done

echo "done -> $OUT"
