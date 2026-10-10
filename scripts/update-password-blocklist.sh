#!/usr/bin/env bash
# パスワードのブロックリスト (設計書 9.3) を、SecLists (MIT) から作り直す。
# 8 文字未満のパスワードは長さの規定で拒否するので含めない。大文字と小文字は区別せずに照合する。
# 使い方: scripts/update-password-blocklist.sh <SecLists のコミット>
set -euo pipefail

commit="${1:?usage: $0 <SecLists commit>}"
base="https://raw.githubusercontent.com/danielmiessler/SecLists/${commit}"
out="$(dirname "$0")/../internal/password"

curl -sSfL "${base}/Passwords/Common-Credentials/xato-net-10-million-passwords-100000.txt" |
  LC_ALL=C awk 'length($0) >= 8' | LC_ALL=C tr 'A-Z' 'a-z' | LC_ALL=C sort -u |
  gzip -9n > "${out}/blocklist.txt.gz"
curl -sSfL "${base}/LICENSE" > "${out}/blocklist.LICENSE"
printf 'https://github.com/danielmiessler/SecLists/blob/%s/Passwords/Common-Credentials/xato-net-10-million-passwords-100000.txt\n' "${commit}" > "${out}/blocklist.SOURCE"
