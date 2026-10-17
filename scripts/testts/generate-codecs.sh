#!/bin/sh
# H.264 と HEVC の映像だけのテスト用の TS を作る。コンテナの中で動かす (scripts/generate-test-ts.sh を参照)。
#
# 索引が I フレームを拾えるかを確かめるためのもので、SI は差し替えない。
# 開いた GOP にして、IDR でない I フレーム (H.264 の I スライス、HEVC の CRA) も含める。
set -eu

out_dir="${1:-/out}"

ffmpeg -hide_banner -loglevel error -y \
  -f lavfi -i "testsrc2=size=320x180:rate=30000/1001" \
  -t 4 \
  -c:v libx264 -preset ultrafast -x264-params "keyint=30:min-keyint=30:scenecut=0:open-gop=1:bframes=3" \
  -fflags +bitexact -flags:v +bitexact \
  -f mpegts -mpegts_start_pid 0x0111 \
  "${out_dir}/test-h264.ts"

ffmpeg -hide_banner -loglevel error -y \
  -f lavfi -i "testsrc2=size=320x180:rate=30000/1001" \
  -t 4 \
  -c:v libx265 -preset ultrafast \
  -x265-params "keyint=30:min-keyint=30:scenecut=0:open-gop=1:bframes=3:repeat-headers=1:aud=1:log-level=error" \
  -fflags +bitexact -flags:v +bitexact \
  -f mpegts -mpegts_start_pid 0x0111 \
  "${out_dir}/test-hevc.ts"
