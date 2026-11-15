#!/usr/bin/env bash
# 「この端末で試す」(#1771、設計書 7.5) で流す、合成の映像を作る。
# 放送の映像は載せられないので、ffmpeg の testsrc2 から作る。作ったファイルはリポジトリに置き、作り直すときだけ動かす。
#
#   MISE_ENV=demo-stream mise run //frontend:benchmark-streams
#
# - interlaced.m2ts: 59.94 枚の絵を、フィールドに分けて載せた 1080i。フィールドごとの表示と、コマごとの表示を試す。
# - film.m2ts: 23.976 枚の絵を、2:3 プルダウンで載せた 1080i。プルダウンを組み直した 24p を試す。
#
# どちらも放送と同じ 1440×1080 (横に縮めた 16:9) の MPEG-2 で、長さは 30 コマ (1.001 秒)。
# ブラウザの中で、時刻を進めながら繰り返して流す (src/yunomi/benchmarkStream.ts)。30 コマは、プルダウンの 5 コマの周期と
# GOP の 15 コマで割り切れるので、つなぎ目でも周期が崩れない。GOP は閉じる。音声は入れない。
#
# 絵は、画面全体を横に流す (1 秒に 96 画素)。止まった所が多いと、フィルムの絵が変わっても違うとみなすブロック
# (yadif は 16 個から) が足りず、yadif がプルダウンを見つけられない。
# 繰り返しの継ぎ目で絵が元の位置へ飛ぶと、見ていて酔うので、幅 96 画素の止まった絵を横に 15 枚並べた模様にし、
# 1 回の繰り返し (1.001 秒) でちょうど 96 画素だけ流す。継ぎ目でも同じ絵につながり、同じ向きに流れ続ける。
# film の絵は、平らな色の面 (smptehdbars) にする。testsrc2 のような細かい絵では、2:3 プルダウンで繰り返すフィールドにも
# 符号化の誤差が乗り、yadif が同じとみなすブロック (違うのが 2 個まで) に収まらなかった。-q:v 3 (約 22 Mbps) にして、
# 手元で同じ基準で数えると同じとみなせても、ブラウザの中では見つけられなかった。平らな色の面なら、13 Mbps でも見つけられる。
# どちらも、放送と同じくらいの 13 Mbps で符号化する。
# ffmpeg は、静的なデモの映像と同じ mise.demo-stream.toml の版を使い、-bitexact で同じファイルを作る。
set -euo pipefail

cd "$(dirname "$0")/.."
mkdir -p public/benchmark

encode() {
  local source="$1" fields="$2" rate="$3" output="$4"
  # ffmpeg の -flags は、後に書いた方で置き換わるので、インターレースの符号化と bitexact を 1 つにまとめる。
  ffmpeg -hide_banner -loglevel error -y \
    -f lavfi -i "${source}" \
    -vf "${fields},setsar=4/3,format=yuv420p,setfield=tff" \
    -frames:v 30 \
    -c:v mpeg2video ${rate} -g 15 -bf 2 -sc_threshold 1000000000 -aspect 16:9 \
    -fflags +bitexact -flags:v +ilme+ildct+cgop+bitexact \
    -f mpegts -mpegts_m2ts_mode 0 "${output}"
}

# tiled は、幅 96 画素の source の最初の絵を止め、横に 15 枚並べて 1440 画素にし、1 枚の絵ごとに step 画素ずつ流す
# lavfi の入力を返す。scroll の速さは、幅に対する割合で渡す。
tiled() {
  local source="$1" rate="$2" step="$3" labels=''
  for i in $(seq 0 14); do labels+="[t${i}]"; done
  echo "${source}=size=96x1080:rate=${rate},trim=end_frame=1,loop=loop=-1:size=1,setpts=N/(${rate})/TB,split=15${labels};${labels}hstack=inputs=15,scroll=horizontal=${step}/1440"
}

# interlaced は、1 回の繰り返しに 60 枚の絵 (フィールド) があるので 1.6 画素ずつ、
# film は、24 枚のフィルムの絵があるので 4 画素ずつ流す。
encode "$(tiled testsrc2 60000/1001 1.6)" 'tinterlace=mode=interleave_top' \
  '-b:v 13M -minrate 13M -maxrate 13M -bufsize 9781248' public/benchmark/interlaced.m2ts
encode "$(tiled smptehdbars 24000/1001 4)" 'telecine=first_field=top:pattern=23' \
  '-b:v 13M -minrate 13M -maxrate 13M -bufsize 9781248' public/benchmark/film.m2ts
ls -l public/benchmark
