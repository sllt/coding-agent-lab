#!/usr/bin/env bash
# Build the web UI and a self-contained agentlab binary with it embedded.
#   scripts/build-web.sh [output]   (default: ./agentlab)
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
out="${1:-$root/agentlab}"
cd "$root/web"
if [ ! -d node_modules ]; then npm ci; fi
npm run build
find "$root/internal/webui/dist" -mindepth 1 ! -name .gitkeep -delete 2>/dev/null || true
mkdir -p "$root/internal/webui/dist"
cp -r "$root/web/dist/." "$root/internal/webui/dist/"
cd "$root"
go build -tags webembed -o "$out" ./cmd/agentlab
echo "built $out with embedded web UI"
