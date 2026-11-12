#!/usr/bin/env bash
# 配るアーカイブを開き、中のバイナリが起動して、画面と埋め込んだファイルを返すことを確かめる (#1523、設計書 11.6)。
# Windows と macOS のランナーで、Linux で作ったスナップショットのアーカイブを確かめるのに使う。手元の Linux でも流せる。
#
# 使い方: KOTATSU_DATABASE_URL=postgres://.../空のDB scripts/ci-smoke-archive.sh <アーカイブ> <確かめる資産のパスの一覧>
#   アーカイブは、GoReleaser が作った kotatsu_<版>_<OS>_<アーキテクチャ>.tar.gz か .zip。
#   資産のパスの一覧は、/assets/... のように、埋め込んだ画面の中のパスを 1 行に 1 つ書いたファイル。
#   EXPECTED_COMMIT を渡すと、--version がそのコミットを返すことも確かめる。
#   KOTATSU_SMOKE_START_TIMEOUT は、起動してからヘルスチェックが ok を返すまで待つ秒 (既定は 120)。
# 偽のチューナーサーバー (backend/cmd/faketuner) を、同じマシンで Go から立てる。
set -euo pipefail

cd "$(dirname "$0")/.."

archive="$1"
assets="$2"
: "${KOTATSU_DATABASE_URL:?KOTATSU_DATABASE_URL must point to an empty database}"
start_timeout="${KOTATSU_SMOKE_START_TIMEOUT:-120}"
port="${KOTATSU_SMOKE_PORT:-18091}"
tuner_port="${KOTATSU_SMOKE_TUNER_PORT:-40774}"
work="$(mktemp -d)"

pids=()
cleanup() {
  # macOS の /bin/bash (3.2) は、set -u のもとで空の配列を展開するとエラーにするので、空なら何も展開しない。
  for pid in ${pids[@]+"${pids[@]}"}; do
    kill "$pid" 2>/dev/null || true
  done
  wait 2>/dev/null || true
  rm -rf "$work"
}
trap cleanup EXIT

step() {
  echo "::group::$1"
  date -u +%T
}

step "Extract the archive"
mkdir -p "$work/archive"
case "$archive" in
  *.zip)
    # Git Bash の tar は zip を開けないので、Windows のランナーにある 7-Zip で開く。
    if command -v 7z >/dev/null; then
      7z x -o"$work/archive" "$archive" >/dev/null
    else
      unzip -q "$archive" -d "$work/archive"
    fi
    ;;
  *.tar.gz) tar -xzf "$archive" -C "$work/archive" ;;
  *)
    echo "unknown archive format: $archive" >&2
    exit 1
    ;;
esac
kotatsu="$work/archive/kotatsu"
[ -f "$kotatsu.exe" ] && kotatsu="$kotatsu.exe"
# artifact を経由すると、実行権限が落ちることがある。
chmod +x "$kotatsu"
for file in LICENSE README.md; do
  test -s "$work/archive/$file"
done
echo "::endgroup::"

step "Check the version"
version="$("$kotatsu" --version)"
echo "$version"
if [ -n "${EXPECTED_COMMIT:-}" ]; then
  grep -F "(${EXPECTED_COMMIT:0:7})" <<<"$version"
fi
echo "::endgroup::"

step "Start the fake tuner server"
go build -o "$work/faketuner" ./backend/cmd/faketuner
"$work/faketuner" -listen "127.0.0.1:$tuner_port" -ts backend/testdata/ts/test.ts >"$work/faketuner.log" 2>&1 &
pids+=("$!")
echo "::endgroup::"

step "Migrate and start Kotatsu"
mkdir -p "$work/recordings" "$work/data" "$work/kyusu"
export KOTATSU_AUTH_SECRET_KEY="kotatsu-smoke-only-secret-key-do-not-use"
export KOTATSU_TUNER_URL="http://127.0.0.1:$tuner_port/"
export KOTATSU_STORAGE_POOLS="main=$work/recordings"
export KOTATSU_STORAGE_DATA_DIR="$work/data"
export KOTATSU_SERVER_LISTEN="127.0.0.1:$port"
export KOTATSU_METRICS_LISTEN="127.0.0.1:0"
export KOTATSU_KYUSU_WORK_DIR="$work/kyusu"
"$kotatsu" migrate
"$kotatsu" all >"$work/kotatsu.log" 2>&1 &
kotatsu_pid="$!"
pids+=("$kotatsu_pid")

started="$(date +%s)"
ready=false
while [ "$(($(date +%s) - started))" -lt "$start_timeout" ]; do
  if ! kill -0 "$kotatsu_pid" 2>/dev/null; then
    break
  fi
  if curl -fsS "http://127.0.0.1:$port/readyz" >/dev/null 2>&1; then
    ready=true
    break
  fi
  sleep 1
done
echo "ready=$ready after $(($(date +%s) - started))s"
if [ "$ready" != true ]; then
  echo "Kotatsu did not become ready within ${start_timeout}s" >&2
  cat "$work/kotatsu.log" >&2
  exit 1
fi
echo "::endgroup::"

step "Check the endpoints and the embedded files"
status=0
check() {
  local code
  code="$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$port$1")"
  echo "$code $1"
  [ "$code" = 200 ] || status=1
}
check /healthz
check /readyz
check /
while IFS= read -r path || [ -n "$path" ]; do
  [ -n "$path" ] && check "$path"
done <"$assets"
echo "::endgroup::"

if [ "$status" -ne 0 ]; then
  echo "--- kotatsu log (tail) ---" >&2
  tail -n 100 "$work/kotatsu.log" >&2
fi
exit "$status"
