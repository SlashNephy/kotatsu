#!/usr/bin/env bash
# イメージに入れた FFmpeg が、docker/ffmpeg/components の部品をすべて持っているかを確かめる (#2001)。
# 依存が足りない部品は、configure が黙って外すので、部品が抜けたことに実行時まで気づけない。
#
#   check-components.sh <components> <アーキテクチャ>
set -euo pipefail

components="$1"
arch="$2"

# 一覧の、区切り (---) より後の行から、1 列目の印が flag に合う行の 2 列目の名前を出す。
list() {
  ffmpeg -hide_banner "$1" | awk -v flag="$2" '/^ *-+$/ { body = 1; next } body && $1 ~ flag { print $2 }'
}

declare -A have
for kind in decoder encoder filter muxer demuxer bsf protocol indev; do
  case "$kind" in
    decoder) names="$(list -decoders '')" ;;
    encoder) names="$(list -encoders '')" ;;
    filter) names="$(list -filters '')" ;;
    muxer) names="$(list -muxers E | tr ',' '\n')" ;;
    demuxer) names="$(list -demuxers D | tr ',' '\n')" ;;
    bsf) names="$(ffmpeg -hide_banner -bsfs | tail -n +2)" ;;
    protocol) names="$(ffmpeg -hide_banner -protocols | awk '/^Input:/ { f = 1; next } /^Output:/ { f = 0 } f { print $1 }')" ;;
    indev) names="$(list -devices D)" ;;
  esac
  for name in $names; do
    have["$kind $name"]=1
  done
done

missing=0
while read -r kind name only; do
  case "$kind" in '' | '#'*) continue ;; esac
  if [ -n "${only:-}" ] && [ "$only" != "$arch" ]; then
    continue
  fi
  case "$kind" in
    parser)
      # パーサーは一覧を出せない。依存がないので、configure が外すこともない。
      continue
      ;;
    hwaccel)
      # mpeg2_vaapi なら、mpeg2video のデコーダーが vaapi のデバイスを使えるかを見る。
      decoder="${name%_*}"
      [ "$decoder" = mpeg2 ] && decoder=mpeg2video
      device="${name##*_}"
      [ "$device" = nvdec ] && device=cuda
      if ffmpeg -hide_banner -h "decoder=$decoder" | grep -A1 'Supported hardware devices' | grep -qw "$device"; then
        continue
      fi
      ;;
    *)
      if [ -n "${have["$kind $name"]:-}" ]; then
        continue
      fi
      ;;
  esac
  echo "missing $kind: $name" >&2
  missing=1
done < "$components"

# ハードウェアのデバイスの種類。
for device in vaapi cuda $([ "$arch" = amd64 ] && echo qsv); do
  if ! ffmpeg -hide_banner -hwaccels | grep -qx "$device"; then
    echo "missing hwaccel device: $device" >&2
    missing=1
  fi
done

# GPL と両立しない部品を組み込んでいないこと (付録 B)。
if ffmpeg -hide_banner -buildconf | grep -q -- '--enable-nonfree'; then
  echo 'FFmpeg is configured with --enable-nonfree' >&2
  missing=1
fi

# 共有ライブラリが、すべて見つかること。
for bin in ffmpeg ffprobe; do
  if ldd "$(command -v "$bin")" | grep 'not found'; then
    echo "$bin has unresolved shared libraries" >&2
    missing=1
  fi
done

exit "$missing"
