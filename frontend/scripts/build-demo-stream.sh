#!/usr/bin/env bash
# 静的なデモ (#832) で、ライブ視聴と録画の再生に流す、64.064 秒の映像を作る (#1050)。
# 映像は、demo/stream-sw.js が繰り返して流すので、つなぎ目で絵も音も途切れないように作る。
#
#   MISE_ENV=demo-stream mise run //frontend:demo-stream
#
# 放送の映像や、権利のはっきりしない素材は載せられないので、すべて自前で作る。素材のファイルは使わない。
# - 絵: 空は ffmpeg の gradients で塗り、山や家、雲、局のロゴ、テロップは scripts/demo-stream-scene.mjs が ASS の図形と文字で描く。
#   局と地名は架空のもの。文字のフォントは、字幕を描くのと同じ Rounded M+ 1m WadaLab mix (vendor/caption-font、Unlicense)。
# - 音: 朝の番組の BGM と小鳥のさえずりを、scripts/demo-stream-music.mjs が式だけで合成する。
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

# 長さは、映像の 1920 フレーム (29.97 fps) と AAC の 3003 フレーム (1024 標本) がちょうど収まる 64.064 秒にする。
# 繰り返すときに、映像と音声の時刻を、どちらも同じ長さだけずらせば済む。

# 音声は、繰り返しの前後に AAC の 1 フレーム分ずつ続きを付けた WAV を、先に AAC (ADTS) にする。
# 符号化器は、頭に遅延の分の 1 フレームを足すので、それを除いた 3003 フレームを取り出し、映像の頭より 1 フレーム前から並べる。
# 取り出した頭のフレームは、前の繰り返しの終わりとのつながりを符号化したものなので、繰り返しても音が途切れない。
node scripts/demo-stream-music.mjs "${work}/music.wav"
ffmpeg -hide_banner -loglevel error -y \
  -i "${work}/music.wav" \
  -c:a aac -b:a 128k -ac 2 -fflags +bitexact -flags:a +bitexact \
  -f adts "${work}/music.aac"
ffmpeg -hide_banner -loglevel error -y \
  -i "${work}/music.aac" -ss 0.0213333 -t 64.064 -c copy -f adts "${work}/loop.aac"

# GOP は閉じ (+cgop)、途中の I フレームから流し始めても、前の GOP なしに復号できるようにする。
# 閉じた GOP は場面の切り替わりの検出と併せられないので、検出を止め (-sc_threshold)、I フレームを 90 フレームごとに揃える。
ffmpeg -hide_banner -loglevel error -y \
  -f lavfi -i 'gradients=size=1920x1080:rate=30000/1001:duration=64.064:c0=0x3b82c4:c1=0xd6ecff:x0=960:y0=0:x1=960:y1=760:speed=0' \
  -itsoffset -0.0213333 -i "${work}/loop.aac" \
  -filter_complex "[0:v]ass=${work}/scene.ass:fontsdir=${work}/fonts,scale=1440:1080,setsar=4/3,format=yuv420p[v]" \
  -map '[v]' -map 1:a \
  -c:v mpeg2video -q:v 6 -g 90 -bf 2 -sc_threshold 1000000000 -aspect 16:9 -frames:v 1920 \
  -c:a copy \
  -fflags +bitexact -flags:v +bitexact+cgop \
  -f mpegts -mpegts_m2ts_mode 0 "${work}/video.ts"

node scripts/demo-stream-captions.mjs "${work}/video.ts" demo/stream.m2ts
ls -l demo/stream.m2ts
