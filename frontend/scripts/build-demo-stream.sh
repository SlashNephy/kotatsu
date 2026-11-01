#!/usr/bin/env bash
# 静的なデモ (#832) で、ライブ視聴と録画の再生に流す、20 秒の映像を作る (#1050)。
#
#   MISE_ENV=demo-stream mise run //frontend:demo-stream
#
# 放送の映像や、権利のはっきりしない素材は載せられないので、すべて自前で作る。素材のファイルは使わない。
# - 絵: 空は ffmpeg の gradients で塗り、山や家、雲、局のロゴ、テロップは scripts/demo-stream-scene.mjs が ASS の図形と文字で描く。
#   局と地名は架空のもの。文字のフォントは、字幕を描くのと同じ Rounded M+ 1m WadaLab mix (vendor/caption-font、Unlicense)。
# - 音: ffmpeg の aevalsrc で、和音を合成する。
# - 字幕: scripts/demo-stream-captions.mjs が、aribb24.js で ARIB 字幕の PES を作り、TS に差し込む。
#
# 放送と同じく、1440x1080 (横に縮めた 16:9) の MPEG-2 の映像と AAC の音声を MPEG-TS に入れる。
# 作った demo/stream.m2ts はリポジトリに置く。作り直すときだけ、このスクリプトを動かす。
# ffmpeg は、mise.demo-stream.toml で版を固定したものを使う。-bitexact を付け、同じ版からは同じファイルができるようにする。
#
# ffmpeg は、出力の拡張子が .m2ts だと、192 バイトのパケット (BDAV) で書く。
# ブラウザ内の変換 (mpeg2toh264) は 188 バイトのパケットしか読まないので、-mpegts_m2ts_mode 0 で止める。
set -euo pipefail

cd "$(dirname "$0")/.."

font="vendor/caption-font/rounded-mplus-1m-wadalab-mix.ttf"
if [[ ! -f "${font}" ]]; then
  echo "${font} is missing; run mise run //frontend:install first" >&2
  exit 1
fi

work="$(mktemp -d)"
trap 'rm -rf "${work}"' EXIT

# ass フィルターは fontsdir のファイルをすべてフォントとして読むので、フォントだけを置いたディレクトリを渡す。
mkdir "${work}/fonts"
cp "${font}" "${work}/fonts/"
node scripts/demo-stream-scene.mjs >"${work}/scene.ass"

# 和音 (ハ長調の I、ド・ミ・ソ) を、ゆっくり揺らして鳴らす。つなぎ目で音が途切れて聞こえないよう、頭と終わりを絞る。
chord='(0.10*sin(2*PI*261.63*t)+0.07*sin(2*PI*329.63*t)+0.06*sin(2*PI*392.00*t)+0.03*sin(2*PI*523.25*t))*(0.8+0.2*sin(2*PI*0.25*t))'

ffmpeg -hide_banner -loglevel error -y \
  -f lavfi -i 'gradients=size=1920x1080:rate=30000/1001:duration=20:c0=0x3b82c4:c1=0xd6ecff:x0=960:y0=0:x1=960:y1=760:speed=0' \
  -f lavfi -i "aevalsrc=exprs=${chord}|${chord}:sample_rate=48000:duration=20" \
  -filter_complex "[0:v]ass=${work}/scene.ass:fontsdir=${work}/fonts,scale=1440:1080,setsar=4/3,format=yuv420p[v];[1:a]afade=t=in:d=0.5,afade=t=out:st=19.5:d=0.5[a]" \
  -map '[v]' -map '[a]' \
  -c:v mpeg2video -q:v 6 -g 90 -bf 2 -aspect 16:9 \
  -c:a aac -b:a 64k -ac 2 \
  -fflags +bitexact -flags:v +bitexact -flags:a +bitexact \
  -f mpegts -mpegts_m2ts_mode 0 "${work}/video.ts"

node scripts/demo-stream-captions.mjs "${work}/video.ts" demo/stream.m2ts
ls -l demo/stream.m2ts
