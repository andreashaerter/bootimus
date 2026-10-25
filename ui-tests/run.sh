#!/usr/bin/env bash
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo="$(cd "$here/.." && pwd)"
admin_port="${BOOTIMUS_UI_ADMIN_PORT:-18081}"
http_port="${BOOTIMUS_UI_HTTP_PORT:-18080}"
tftp_port="${BOOTIMUS_UI_TFTP_PORT:-16969}"
bin="${BOOTIMUS_UI_BINARY:-$repo/.ui-tests/bootimus}"
data="$(mktemp -d "${TMPDIR:-/tmp}/bootimus-ui-XXXXXX")"
admin_password="ui-admin-$(head -c 12 /dev/urandom | od -An -tx1 | tr -d ' \n')"
viewer_password="ui-viewer-$(head -c 12 /dev/urandom | od -An -tx1 | tr -d ' \n')"
server_pid=""

cleanup() {
  if [ -n "$server_pid" ] && kill -0 "$server_pid" 2>/dev/null; then
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
  if [ -z "${BOOTIMUS_UI_KEEP_DATA:-}" ]; then
    rm -rf "$data"
  else
    echo "data directory kept at $data"
  fi
}
trap cleanup EXIT

if [ ! -x "$bin" ]; then
  mkdir -p "$(dirname "$bin")"
  (cd "$repo" && go build -o "$bin" .)
fi

"$bin" serve \
  --data-dir "$data" \
  --server-addr 127.0.0.1 \
  --tftp-port "$tftp_port" \
  --http-port "$http_port" \
  --admin-port "$admin_port" \
  --nbd-enabled=false \
  >"$data/server.log" 2>&1 &
server_pid=$!

for _ in $(seq 1 60); do
  if curl -sf "http://127.0.0.1:$admin_port/health" >/dev/null 2>&1; then
    break
  fi
  if ! kill -0 "$server_pid" 2>/dev/null; then
    echo "bootimus exited during startup:" >&2
    cat "$data/server.log" >&2
    exit 1
  fi
  sleep 0.5
done
curl -sf "http://127.0.0.1:$admin_port/health" >/dev/null

"$bin" user set-password admin --password "$admin_password" --data-dir "$data" >/dev/null

token="$(curl -sf -X POST "http://127.0.0.1:$admin_port/api/login" \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"admin\",\"password\":\"$admin_password\"}" \
  | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')"
if [ -z "$token" ]; then
  echo "could not log in as admin; server log follows:" >&2
  cat "$data/server.log" >&2
  exit 1
fi

curl -sf -X POST "http://127.0.0.1:$admin_port/api/users" \
  -H "Authorization: Bearer $token" \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"viewer\",\"password\":\"$viewer_password\",\"is_admin\":false}" >/dev/null

export BOOTIMUS_UI_URL="http://127.0.0.1:$admin_port"
export BOOTIMUS_UI_ADMIN_USER="admin"
export BOOTIMUS_UI_ADMIN_PASSWORD="$admin_password"
export BOOTIMUS_UI_VIEWER_USER="viewer"
export BOOTIMUS_UI_VIEWER_PASSWORD="$viewer_password"

status=0
(cd "$here" && npx playwright test "$@") || status=$?

if [ "$status" -ne 0 ]; then
  echo "server log tail:" >&2
  tail -n 60 "$data/server.log" >&2
fi
exit "$status"
