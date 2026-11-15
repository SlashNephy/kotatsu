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
# 絵は、画面全体を横に流す (1 秒に約 96 画素)。止まった所が多いと、フィルムの絵が変わっても違うとみなすブロック
# (yadif は 16 個から) が足りず、yadif がプルダウンを見つけられない。
# interlaced は、放送並みの 13 Mbps にする。film は、画質を -q:v 3 (約 22 Mbps) にする。放送並みに絞ると、
# 2:3 プルダウンで繰り返すフィールドにも符号化の誤差が乗り、同じとみなすブロック (違うのが 2 個まで) に収まらず、
# プルダウンを見つけられない。film は放送より重めの映像なので、変換の負荷は放送より少し大きく出る。
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

encode 'testsrc2=size=1440x1080:rate=60000/1001,scroll=horizontal=0.0011' 'tinterlace=mode=interleave_top' \
  '-b:v 13M -minrate 13M -maxrate 13M -bufsize 9781248' public/benchmark/interlaced.m2ts
encode 'smptehdbars=size=1440x1080:rate=24000/1001,scroll=horizontal=0.0028' 'telecine=first_field=top:pattern=23' \
  '-q:v 3' public/benchmark/film.m2ts
ls -l public/benchmark
