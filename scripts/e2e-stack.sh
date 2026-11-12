#!/usr/bin/env bash
# 偽のチューナーサーバー、偽の OIDC の IdP と Kotatsu (全ロール) を立て、frontend/e2e/stack の E2E テストを流す (#508、#1525、設計書 11.6)。
# Kotatsu は、ローカルのログインと OIDC を併用する。
#
# 使い方: KOTATSU_DATABASE_URL=postgres://.../空のDB scripts/e2e-stack.sh [playwright test の引数...]
# 先に mise run //frontend:build と mise run build で、画面を埋め込んだ ./bin/kotatsu を作っておく。
# DB は空にしておく。初回セットアップから流すので、管理者がいると失敗する。
set -euo pipefail

cd "$(dirname "$0")/.."

: "${KOTATSU_DATABASE_URL:?KOTATSU_DATABASE_URL must point to an empty database}"
port="${KOTATSU_E2E_PORT:-18090}"
tuner_port="${KOTATSU_E2E_TUNER_PORT:-40773}"
idp_port="${KOTATSU_E2E_IDP_PORT:-15557}"
idp_control_port="${KOTATSU_E2E_IDP_CONTROL_PORT:-15558}"
work="$(mktemp -d)"
log="${KOTATSU_E2E_LOG:-$work/kotatsu.log}"

pids=()
cleanup() {
  for pid in "${pids[@]}"; do
    kill "$pid" 2>/dev/null || true
  done
  wait 2>/dev/null || true
  rm -rf "$work"
}
trap cleanup EXIT

go build -o "$work/faketuner" ./backend/cmd/faketuner
"$work/faketuner" -listen "127.0.0.1:$tuner_port" -ts backend/testdata/ts/test.ts >"$work/faketuner.log" 2>&1 &
pids+=("$!")

# 偽の IdP。グループは、自動で作るユーザーのロールを決めるのに使う。制御用のエンドポイントは、テストが次のユーザーを変えるのに使う。
oidc_secret="kotatsu-e2e-only-oidc-secret"
go build -o "$work/fakeidp" ./backend/cmd/fakeidp
"$work/fakeidp" -listen "127.0.0.1:$idp_port" -control "127.0.0.1:$idp_control_port" \
  -client-id kotatsu -client-secret "$oidc_secret" \
  -redirect-uri "http://127.0.0.1:$port/auth/oidc/callback" -groups kotatsu-members >"$work/fakeidp.log" 2>&1 &
pids+=("$!")

mkdir -p "$work/recordings" "$work/data"
export KOTATSU_AUTH_SECRET_KEY="kotatsu-e2e-only-secret-key-do-not-use"
export KOTATSU_TUNER_URL="http://127.0.0.1:$tuner_port/"
export KOTATSU_STORAGE_POOLS="main=$work/recordings"
export KOTATSU_STORAGE_DATA_DIR="$work/data"
export KOTATSU_SERVER_LISTEN="127.0.0.1:$port"
export KOTATSU_METRICS_LISTEN="127.0.0.1:0"
export KOTATSU_KYUSU_WORK_DIR="$work/kyusu"
export KOTATSU_AUTH_OIDC_ISSUER="http://127.0.0.1:$idp_port"
export KOTATSU_AUTH_OIDC_CLIENT_ID=kotatsu
export KOTATSU_AUTH_OIDC_CLIENT_SECRET="$oidc_secret"
export KOTATSU_AUTH_OIDC_REDIRECT_URL="http://127.0.0.1:$port/auth/oidc/callback"
export KOTATSU_AUTH_OIDC_DISPLAY_NAME="E2E の IdP"
export KOTATSU_AUTH_OIDC_AUTO_CREATE_ENABLED=true
export KOTATSU_AUTH_OIDC_AUTO_CREATE_GROUPS_CLAIM=groups
export KOTATSU_AUTH_OIDC_AUTO_CREATE_ADMIN_GROUPS=kotatsu-admins
export KOTATSU_AUTH_OIDC_AUTO_CREATE_MEMBER_GROUPS=kotatsu-members
export KOTATSU_AUTH_OIDC_AUTO_CREATE_VIEWER_GROUPS=kotatsu-viewers

./bin/kotatsu migrate
./bin/kotatsu all >"$log" 2>&1 &
kotatsu_pid="$!"
pids+=("$kotatsu_pid")

# 起動と、セットアップトークンのログを待つ。落ちたら、待たずに失敗する。
token=""
ready=false
for _ in $(seq 1 120); do
  if ! kill -0 "$kotatsu_pid" 2>/dev/null; then
    break
  fi
  token="$(grep -o '"setup_token":"[^"]*"' "$log" | head -n 1 | cut -d '"' -f 4 || true)"
  if [ -n "$token" ] && curl -fsS "http://127.0.0.1:$port/readyz" >/dev/null 2>&1; then
    ready=true
    break
  fi
  sleep 1
done
if [ "$ready" != true ]; then
  echo "Kotatsu did not become ready with a setup token" >&2
  cat "$log" >&2
  exit 1
fi

status=0
(
  cd frontend
  KOTATSU_E2E_URL="http://127.0.0.1:$port" KOTATSU_E2E_SETUP_TOKEN="$token" \
    KOTATSU_E2E_IDP_CONTROL_URL="http://127.0.0.1:$idp_control_port" \
    pnpm exec playwright test --project=stack --project=stack-oidc "$@"
) || status=$?
if [ "$status" -ne 0 ]; then
  echo "--- kotatsu log (tail) ---" >&2
  tail -n 100 "$log" >&2
fi
exit "$status"
