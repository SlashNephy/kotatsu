#!/bin/sh
# テスト用の TS を作る。コンテナの中で動かす (backend/scripts/generate-test-ts.sh を参照)。
#
# FFmpeg でテストパターンの映像 (MPEG-2) とサイン波の音声 (AAC) を作り、TSDuck で SI を差し替える。
# 実際の放送の TS は使わない。
set -eu

out="${1:-/out/test.ts}"
raw="$(mktemp --suffix=.ts)"
trap 'rm -f "${raw}"' EXIT

# 固定のビットレート (-muxrate) にして、SI を差し込むための null パケットを確保する。
ffmpeg -hide_banner -loglevel error -y \
  -f lavfi -i "testsrc2=size=720x480:rate=30000/1001" \
  -f lavfi -i "sine=frequency=1000:sample_rate=48000" \
  -t 6 \
  -c:v mpeg2video -b:v 1000k -maxrate 1000k -bufsize 1835k -g 15 \
  -c:a aac -b:a 128k -ac 2 \
  -fflags +bitexact -flags:v +bitexact -flags:a +bitexact \
  -f mpegts -muxrate 1500k \
  -mpegts_original_network_id 0x7FE0 -mpegts_transport_stream_id 0x7FE0 -mpegts_service_id 0x0400 \
  -mpegts_pmt_start_pid 0x01F0 -mpegts_start_pid 0x0111 \
  "${raw}"

# FFmpeg の SDT を捨て、ARIB の文字符号で書いた SDT、EIT[p/f]、TOT を差し込む。
tsp \
  -I file "${raw}" \
  -P filter --negate --pid 0x11 \
  -P inject --pid 0x11 --bitrate 15000 --default-charset ARIB-STD-B24 /work/tables/sdt.xml \
  -P inject --pid 0x12 --bitrate 15000 --default-charset ARIB-STD-B24 /work/tables/eit.xml \
  -P inject --pid 0x14 --bitrate 3000 /work/tables/tot.xml \
  -O file "${out}"

tstables --default-charset ARIB-STD-B24 --pid 0x11 --pid 0x12 --max-tables 2 "${out}"
