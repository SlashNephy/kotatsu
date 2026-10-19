#!/usr/bin/env bash
# 静的なデモ (#832) で、ライブ視聴と録画の再生に流す、20 秒の合成の映像を作る。
# 放送の映像は載せられないので、ffmpeg のテストパターン (testsrc) と正弦波から作る。
# 放送と同じく、MPEG-2 の映像と AAC の音声を MPEG-TS に入れる。
# 作った demo/stream.m2ts はリポジトリに置く。作り直すときだけ、このスクリプトを動かす。
#
# ffmpeg は、出力の拡張子が .m2ts だと、192 バイトのパケット (BDAV) で書く。
# ブラウザ内の変換 (mpeg2toh264) は 188 バイトのパケットしか読まないので、-mpegts_m2ts_mode 0 で止める。
set -euo pipefail

cd "$(dirname "$0")/.."
ffmpeg -hide_banner -loglevel error -y \
  -f lavfi -i 'testsrc=size=640x360:rate=30000/1001:duration=20' \
  -f lavfi -i 'sine=frequency=440:sample_rate=48000:duration=20' \
  -c:v mpeg2video -q:v 12 -g 90 -bf 2 -pix_fmt yuv420p \
  -c:a aac -b:a 32k -ac 2 \
  -f mpegts -mpegts_m2ts_mode 0 demo/stream.m2ts
