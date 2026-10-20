#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
data="$(mktemp -d)"
mkdir -p "$data/imports"
cp demos/multi-agent-todo/todo-demo.opsbuddy-project.json "$data/imports/"
# Own data directory, display and D-Bus session. No real model calls.
OPSBUDDY_SMOKE_IMPORT_DEMO=1 timeout 120s dbus-run-session -- xvfb-run -a \
  ./release/linux/opsbuddy --smoke-test --data "$data" >"$data/smoke.log" 2>&1 || {
    cat "$data/smoke.log"; test ! -f "$data/smoke-result.json" || cat "$data/smoke-result.json"; exit 1;
  }
node -e 'const fs=require("fs"); const p=JSON.parse(fs.readFileSync(process.argv[1])); console.log(JSON.stringify(p,null,2)); if(!p.passed || !p.projectImport?.goHealth) process.exit(1)' "$data/smoke-result.json"
