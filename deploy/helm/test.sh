#!/usr/bin/env bash
# Helm Chart の検査 (#1526、設計書 11.1)。
# - tests/fail の values は、Chart の fail の検査で描画が止まる (helm template が非 0 で終わる) はずのもの。描けてしまえば落とす。
# - tests/render の values は、--show-only の対象 (ファイル名と同じテンプレート) を描いた結果が、*.expected.yaml と一致するはずのもの。
set -euo pipefail

dir="$(cd "$(dirname "$0")" && pwd)"
chart="$dir/kotatsu"
status=0

for values in "$dir"/tests/fail/*.yaml; do
  if helm template kotatsu "$chart" -f "$values" >/dev/null 2>&1; then
    echo "FAIL: rendered although it should fail: ${values#"$dir"/}"
    status=1
  else
    echo "ok: ${values#"$dir"/}"
  fi
done

for values in "$dir"/tests/render/*.yaml; do
  case "$values" in *.expected.yaml) continue ;; esac
  name="$(basename "$values" .yaml)"
  if diff -u "${values%.yaml}.expected.yaml" <(helm template kotatsu "$chart" -f "$values" --show-only "templates/$name.yaml"); then
    echo "ok: ${values#"$dir"/}"
  else
    echo "FAIL: unexpected output: ${values#"$dir"/}"
    status=1
  fi
done

exit "$status"
