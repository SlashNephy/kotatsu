#!/bin/sh
# 入れた Debian のパッケージと、そのソースパッケージの名前と版を、タブ区切りで $1 に書き出す。
# GPL のソースの入手先として、版を固定して取得できる snapshot.debian.org の URL を添える (設計書の付録 B)。
set -eu

out="$1"
{
  printf 'package\tversion\tsource\tsource_version\tsource_url\n'
  dpkg-query -W -f '${binary:Package}\t${Version}\t${source:Package}\t${source:Version}\n' |
    awk -F '\t' '{ printf "%s\t%s\t%s\t%s\thttps://snapshot.debian.org/package/%s/%s/\n", $1, $2, $3, $4, $3, $4 }'
} > "$out"
