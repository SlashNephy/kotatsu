#!/usr/bin/env bash
# 配るアーカイブを開き、中のバイナリが起動して、画面と埋め込んだファイルを返すことを確かめる (#1523、設計書 11.6)。
# Windows と macOS のランナーで、Linux で作ったスナップショットのアーカイブを確かめるのに使う。手元の Linux でも流せる。
#
# 使い方: KOTATSU_DATABASE_URL=postgres://.../空のDB scripts/ci-smoke-archive.sh <アーカイブ> <確かめる資産のパスの一覧>
#   アーカイブは、GoReleaser が作った kotatsu_<版>_<OS>_<アーキテクチャ>.tar.gz か .zip。
#   資産のパスの一覧は、/assets/... のように、埋め込んだ画面の中のパスを 1 行に 1 つ書いたファイル。
#   EXPECTED_COMMIT を渡すと、--version がそのコミットを返すことも確かめる。
#   KOTATSU_SMOKE_START_TIMEOUT は、起動してからヘルスチェックが ok を返すまで待つ秒 (既定は 120)。
#   KOTATSU_SMOKE_SLEEP に windows か macos を渡すと、スリープの抑止と起こす予約が OS に届き、外せることも確かめる (#1522、設計書 11.7)。
#     KOTATSU_SMOKE_PSQL に psql のパスを渡す。Windows は管理者で、macOS は sudo で Kotatsu を動かす (起こす予約に要る)。
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
sleep_os="${KOTATSU_SMOKE_SLEEP:-}"
# macOS の起こす予約には root が要るので、Kotatsu を sudo で動かす。root のプロセスは、止めるのも sudo で行う。
as_root=""
if [ "$sleep_os" = macos ]; then
  as_root=sudo
fi

pids=()
cleanup() {
  # macOS の /bin/bash (3.2) は、set -u のもとで空の配列を展開するとエラーにするので、空なら何も展開しない。
  for pid in ${pids[@]+"${pids[@]}"}; do
    $as_root kill "$pid" 2>/dev/null || true
  done
  wait 2>/dev/null || true
  $as_root rm -rf "$work"
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
if [ -n "$as_root" ]; then
  # sudo -E で、ここまでに export した設定を渡す。sudo は、同じプロセスグループから届いたシグナルを中継しないので、
  # 止めるときは sudo でなく Kotatsu そのものに送る。exec で PID を引き継ぐので、シェルが書いた PID が Kotatsu の PID になる。
  # ログは root で書くので、リダイレクトも sudo の中で行う。
  sudo -E sh -c 'echo "$$" >"$1"; log="$2"; shift 2; exec "$@" >"$log" 2>&1' sh "$work/kotatsu.pid" "$work/kotatsu.log" "$kotatsu" all &
  pids+=("$!")
  for _ in $(seq 1 20); do
    [ -s "$work/kotatsu.pid" ] && break
    sleep 0.5
  done
  kotatsu_pid="$(cat "$work/kotatsu.pid")"
else
  "$kotatsu" all >"$work/kotatsu.log" 2>&1 &
  kotatsu_pid="$!"
fi
pids+=("$kotatsu_pid")

started="$(date +%s)"
ready=false
while [ "$(($(date +%s) - started))" -lt "$start_timeout" ]; do
  if ! $as_root kill -0 "$kotatsu_pid" 2>/dev/null; then
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

# wait_for は、"$@" が成り立つまで、判定の間隔 (30 秒) の 3 回分まで待つ。
wait_for() {
  local description="$1"
  shift
  local started
  started="$(date +%s)"
  while [ "$(($(date +%s) - started))" -lt 90 ]; do
    if "$@"; then
      echo "ok: $description after $(($(date +%s) - started))s"
      return 0
    fi
    sleep 2
  done
  echo "timed out: $description" >&2
  status=1
  return 1
}

# Git Bash は、/ で始まる引数をパスに書き換えるので、Windows のコマンドには書き換えずに渡す。
windows_command() {
  MSYS_NO_PATHCONV=1 MSYS2_ARG_CONV_EXCL='*' "$@"
}

# pipefail のもとで grep -q に流すと、grep が先に終えたときに書き手が失敗し、一致しても偽になるので、出力を変数に取ってから探す。
inhibited() {
  local out
  case "$sleep_os" in
    # powercfg /requests は、管理者でないと読めない。
    windows)
      out="$(windows_command powercfg /requests)" || return 1
      grep -qi 'kotatsu\.exe' <<<"$out"
      ;;
    macos)
      out="$(pmset -g assertions)" || return 1
      grep -q 'Kotatsu recording' <<<"$out"
      ;;
  esac
}

wake_scheduled() {
  local out
  case "$sleep_os" in
    windows) windows_command schtasks /Query /TN '\Kotatsu\Wake' >/dev/null 2>&1 ;;
    macos)
      out="$(pmset -g sched)" || return 1
      grep -q 'Kotatsu recording' <<<"$out"
      ;;
  esac
}

not_inhibited() { ! inhibited; }
not_wake_scheduled() { ! wake_scheduled; }

show_sleep_state() {
  case "$sleep_os" in
    windows)
      windows_command powercfg /requests || true
      windows_command schtasks /Query /TN '\Kotatsu\Wake' /V /FO LIST || true
      ;;
    macos)
      pmset -g assertions || true
      pmset -g sched || true
      ;;
  esac
}

if [ -n "$sleep_os" ]; then
  step "Check the sleep control"
  : "${KOTATSU_SMOKE_PSQL:?KOTATSU_SMOKE_PSQL must point to psql}"
  psql_exec() {
    "$KOTATSU_SMOKE_PSQL" "$KOTATSU_DATABASE_URL" -v ON_ERROR_STOP=1 -qAt -c "$1"
  }
  # 9 分後に始まる予約を置く。録画する予定の開始 (開始マージンの既定 15 秒を引いた時刻) は、抑止の前倒し (10 分) の内にあり、
  # 起こす時刻 (5 分前) は、まだ過ぎていない。neko が割り当てを計算し直しても、同じ範囲に収まる。
  reservation_id=00000000-0000-4000-8000-000000001522
  psql_exec "INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', 'GR', 10) ON CONFLICT DO NOTHING"
  psql_exec "INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
    VALUES (6553400001, 65534, 1, 'smoke', 'smoke', 1, 'GR', '99', now(), now()) ON CONFLICT DO NOTHING"
  psql_exec "INSERT INTO reservations (id, kind, service_id, event_id, start_at, end_at, name, description, extended, genres,
    enabled, follow_event_relay, settings, created_at, updated_at, assignment_state, planned_start_at, planned_end_at)
    VALUES ('$reservation_id', 'time', 6553400001, NULL, now() + interval '9 minutes', now() + interval '69 minutes', 'smoke', '', '{}', '[]',
    true, false, '{}', now(), now(), 'ok', now() + interval '9 minutes' - interval '15 seconds', now() + interval '69 minutes')"
  wait_for "the system is kept awake" inhibited || true
  wait_for "a wake is scheduled" wake_scheduled || true
  show_sleep_state
  # 予約を消すと、次の判定で、抑止を外し、起こす予約を消す。
  psql_exec "DELETE FROM reservations WHERE id = '$reservation_id'"
  wait_for "the system is allowed to sleep" not_inhibited || true
  wait_for "the wake is cancelled" not_wake_scheduled || true
  show_sleep_state
  echo "::endgroup::"
fi

if [ "$status" -ne 0 ]; then
  echo "--- kotatsu log (tail) ---" >&2
  tail -n 100 "$work/kotatsu.log" >&2
fi
exit "$status"
