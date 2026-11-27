#!/usr/bin/env bash
# 録画中に mikan を入れ替える場面を、neko と複数の mikan を別のプロセスで立てて確かめる (#2195、設計書 11.6)。
# 引き継ぎの瞬間に録画設定を読めない場面と、古いバイナリの mikan と新しいスキーマの DB が重なる場面を流す。
#
# 使い方: KOTATSU_DATABASE_URL=postgres://.../postgres scripts/e2e-recording.sh [e2erecording の引数...]
# KOTATSU_DATABASE_URL には、DB を作り直せるユーザーで接続する。場面ごとに kotatsu_e2e_* の DB を作り直す。
# 先に mise run build で ./bin/kotatsu を作っておく。
set -euo pipefail

cd "$(dirname "$0")/.."

: "${KOTATSU_DATABASE_URL:?KOTATSU_DATABASE_URL must point to a PostgreSQL where databases can be created}"
work="$(mktemp -d)"
cleanup() {
  if [ "${KOTATSU_E2E_KEEP:-}" = "" ]; then
    rm -rf "$work"
  fi
}
trap cleanup EXIT

# 作業フォルダの外では、mise が別の版の Go を選ぶので、古いバイナリも、リポジトリで選んだ Go で作る。
goroot="$(go env GOROOT)"

# 古いバイナリは、HEAD のソースから、最新のマイグレーションを外して作る。最新のマイグレーションを知らないほかは、同じバイナリである。
mkdir -p "$work/old"
git archive HEAD backend | tar -x -C "$work/old"
latest="$(find "$work/old/backend/db/migrations" -name '*.sql' | sort | tail -n 1)"
rm "$latest"
GOROOT="$goroot" GOTOOLCHAIN=local "$goroot/bin/go" -C "$work/old/backend" build -trimpath -o "$work/kotatsu-old" ./cmd/kotatsu

go -C backend build -o "$work/e2erecording" ./internal/cmd/e2erecording
status=0
"$work/e2erecording" -kotatsu ./bin/kotatsu -old-kotatsu "$work/kotatsu-old" -ts backend/testdata/ts/test.ts -work "$work/runs" "$@" || status=$?
if [ "$status" -ne 0 ]; then
  for log in "$work"/runs/*/*.log; do
    echo "--- $log (tail) ---" >&2
    tail -n 50 "$log" >&2
  done
fi
exit "$status"
