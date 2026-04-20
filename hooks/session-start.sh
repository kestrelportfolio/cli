#!/usr/bin/env bash
# Kestrel plugin session-start hook
#
# Runs at the start of every Claude Code session.
# Tells Claude whether the CLI is installed and authenticated.

set -euo pipefail

if ! command -v kestrel &>/dev/null; then
  cat << 'EOF'
<hook-output>
Kestrel plugin active — CLI not found on PATH.
Install: curl -fsSL https://kestrelportfolio.com/install-cli | bash
Or: brew install kestrelportfolio/tap/kestrel
</hook-output>
EOF
  exit 0
fi

# Check auth by calling kestrel me
me_json=$(kestrel me --json 2>/dev/null || echo '{}')

if echo "$me_json" | grep -q '"ok": true'; then
  cat << 'EOF'
<hook-output>
Kestrel plugin active.
</hook-output>
EOF
else
  cat << 'EOF'
<hook-output>
Kestrel plugin active — not authenticated.
Run: kestrel login
</hook-output>
EOF
fi
