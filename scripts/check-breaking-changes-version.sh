#!/usr/bin/env bash
# 破壊的変更 (docs/breaking-changes.md) の先頭の版の見出しが、これから打つ版と合っているかを確かめる (#1438)。
# 見出しは書いた時点の次の版にするので、月をまたいでリリースすると、打つ版とずれる。ずれていたら、タグを打つ前に止める。
# 先頭の見出しが既にリリースした版で、その節がタグの時点から変わっていなければ、まだリリースしていない破壊的変更はないので、通す。
# 節が変わっていれば、リリースした後にマージした変更が、リリース済みの版の節に入っているので、止める。
#
# 使い方: check-breaking-changes-version.sh <これから打つ版 (YYYY.M.N)> [ファイル]
#   既にある v で始まるタグを、標準入力から 1 行ずつ読む。リポジトリのルートで実行する。
set -euo pipefail

version="${1:?usage: check-breaking-changes-version.sh <version> [file]}"
file="${2:-docs/breaking-changes.md}"

# 先に標準入力を読み切る。途中で終えると、書き手のコマンドが SIGPIPE で落ちることがある。
tags=()
# 最後の行に改行がなくても読む。
while IFS= read -r tag || [[ -n "$tag" ]]; do
  tags+=("$tag")
done

heading="$(grep -m 1 -E '^## v[0-9]' "$file" | sed -E 's/^## (v[^[:space:]]+).*$/\1/' || true)"
if [[ -z "$heading" ]]; then
  echo "no version heading found in $file" >&2
  exit 1
fi

# 先頭の節 (先頭の版の見出しから、次の版の見出しの前まで) を出す。
latest_section() {
  awk '/^## v[0-9]/ { n++ } n == 1 { print }'
}

for tag in "${tags[@]}"; do
  if [[ "$tag" == "$heading" ]]; then
    if ! released="$(git show "${tag}:${file}" 2>/dev/null)"; then
      echo "$file: the latest section is $heading, but $file does not exist at $tag. Rename the heading to v${version} before tagging." >&2
      exit 1
    fi
    if [[ "$(latest_section < "$file")" != "$(latest_section <<< "$released")" ]]; then
      echo "$file: the section $heading has changed since it was released. Move the unreleased changes to a new section v${version} before tagging." >&2
      exit 1
    fi
    echo "$file: the latest section $heading is already released and unchanged; no unreleased breaking changes"
    exit 0
  fi
done

if [[ "$heading" != "v${version}" ]]; then
  echo "$file: the latest section is $heading, but the next release is v${version}. Rename the heading to v${version} before tagging." >&2
  exit 1
fi

echo "$file: the latest section matches v${version}"
