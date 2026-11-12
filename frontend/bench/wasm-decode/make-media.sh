#!/usr/bin/env bash
# 測定に使う合成の 1080i (59.94 フィールド/秒、TFF) の MPEG-2 TS を、.work/media/ に作る (#1516)。
# 絵柄 3 種類 × 幅 2 種類の 30 秒と、testsrc2_1440 からパケットを割合で落とした TS を作る。作ってあるものは作り直さない。
#
# どれも動く雑音を強めに足しているので、ビットレートは指定の上限近くに張り付く。cellauto は上限に収まらず、放送より重い。
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
media="$here/.work/media"
mkdir -p "$media"

declare -A src=(
  [testsrc2]="testsrc2=s=1920x1080:r=60000/1001"
  [mandel]="mandelbrot=s=1920x1080:r=60000/1001:maxiter=200:end_scale=0.001"
  [cellauto]="cellauto=s=480x270:r=60000/1001:rule=110:random_fill_ratio=0.5,scale=1920:1080:flags=neighbor"
)
for name in testsrc2 mandel cellauto; do
  for width in 1920 1440; do
    out="$media/${name}_${width}.ts"
    if [ -f "$out" ]; then continue; fi
    if [ "$width" = 1920 ]; then bitrate=20M sar=1; else bitrate=15M sar=4/3; fi
    ffmpeg -hide_banner -loglevel error -y \
      -f lavfi -i "${src[$name]},noise=alls=12:allf=t,format=yuv420p" \
      -f lavfi -i "sine=f=440:r=48000,aformat=channel_layouts=stereo" \
      -t 30 \
      -vf "scale=$width:1080,tinterlace=interleave_top,setfield=tff,setsar=$sar" \
      -c:v mpeg2video -b:v "$bitrate" -maxrate 24M -bufsize 9781248 \
      -g 15 -bf 2 -flags +ilme+ildct -color_primaries bt709 -color_trc bt709 -colorspace bt709 \
      -c:a aac -b:a 192k -ar 48000 \
      -f mpegts -mpegts_service_id 1024 "$out.tmp"
    mv "$out.tmp" "$out"
  done
done

for rate in 0.0001 0.001 0.01; do
  out="$media/loss_$rate.ts"
  if [ -f "$out" ]; then continue; fi
  node "$here/drop-packets.mjs" "$media/testsrc2_1440.ts" "$out" "$rate" 1516
done
ls -l "$media"
