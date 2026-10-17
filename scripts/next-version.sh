#!/usr/bin/env bash
# 次のリリースの版を、CalVer の YYYY.M.N (年.月.その月の何回目、0 から) で出す (#699)。
# Helm の Chart の版とイメージのタグは SemVer の形が要るので、先頭の 0 を付けず、3 つの数字にする。
#
# 使い方: next-version.sh [YYYY.M]
#   YYYY.M を省くと、今の年と月 (Asia/Tokyo) を使う。既にある v で始まるタグを、標準入力から 1 行ずつ読む。
set -euo pipefail

month="${1:-$(TZ=Asia/Tokyo date +%Y.%-m)}"
if ! [[ "$month" =~ ^[0-9]{4}\.[1-9][0-9]?$ ]]; then
  echo "invalid month: $month (expected YYYY.M)" >&2
  exit 1
fi

next=0
while IFS= read -r tag; do
  if [[ "$tag" =~ ^v${month//./\\.}\.([0-9]+)$ ]]; then
    n="${BASH_REMATCH[1]}"
    if (( n + 1 > next )); then
      next=$(( n + 1 ))
    fi
  fi
done

echo "${month}.${next}"
