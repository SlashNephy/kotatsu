#!/bin/sh
# H.264 と HEVC のテスト用の TS を作る。コンテナの中で動かす (backend/scripts/generate-test-ts.sh を参照)。
#
# 8 ビットの H.264 と HEVC は、索引が I フレームを拾えるかを確かめるためのもの。どれも SI は差し替えない。
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

# BS4K に似せた、10 ビットで 59.94p のプログレッシブの HEVC と、ステレオの AAC の TS。kyusu が 10 ビットの入力を
# ブラウザで再生できる 8 ビットの映像にするかを確かめる。色は BS4K の HLG (BT.2020) にする。
ffmpeg -hide_banner -loglevel error -y \
  -f lavfi -i "testsrc2=size=320x180:rate=60000/1001" \
  -f lavfi -i "sine=frequency=1000:sample_rate=48000" \
  -t 4 \
  -map 0:v -map 1:a \
  -pix_fmt yuv420p10le \
  -c:v libx265 -preset ultrafast -profile:v main10 \
  -x265-params "keyint=30:min-keyint=30:scenecut=0:open-gop=1:bframes=3:repeat-headers=1:aud=1:log-level=error:colorprim=bt2020:transfer=arib-std-b67:colormatrix=bt2020nc" \
  -c:a aac -b:a 128k -ac 2 \
  -fflags +bitexact -flags:v +bitexact -flags:a +bitexact \
  -f mpegts -mpegts_start_pid 0x0111 \
  "${out_dir}/test-hevc-main10.ts"
