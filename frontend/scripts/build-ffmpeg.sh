#!/usr/bin/env bash
# ブラウザ内でデコードする経路 (#1516、設計書 7.2) の WASM の FFmpeg を、frontend/vendor/ffmpeg/SOURCE で固定したバージョンから組み、
# frontend に置く。
#
# FFmpeg を mpeg2video と AAC のデコーダ、mpegvideo の parser だけに絞り、vendor/ffmpeg/glue.c とつないで emscripten で組む。
# 手書きアセンブリと pthreads は使わない (crossOriginIsolated が要らないようにする)。組むのは、SOURCE で固定した
# emscripten のイメージの中で行う。FFmpeg の tarball は、SOURCE の公開鍵で署名を検証してから使う。
# 同じバージョンからは同じものができるので、CI は、組み直したものとリポジトリのものに差分がないことを確認する。
#
# 置くもの:
#   frontend/vendor/ffmpeg/decoder/dist/decoder.js  emscripten が作る、WASM を読み込む JS (LGPL-2.1-or-later)
#   frontend/vendor/ffmpeg/decoder/COPYING.LGPLv2.1 FFmpeg のライセンス文書 (/about の一覧が拾う)
#   frontend/public/ffmpeg/decoder.wasm             デコーダの WebAssembly (LGPL-2.1-or-later)。明示的な URL で配信する
#   frontend/public/ffmpeg/COPYING.LGPLv2.1         FFmpeg のライセンス文書
#   frontend/public/ffmpeg/SOURCE                   ソースの入手先と、組み方
set -euo pipefail

frontend="$(cd "$(dirname "$0")/.." && pwd)"
vendor="$frontend/vendor/ffmpeg"
public="$frontend/public/ffmpeg"
value() { sed -n "s/^$1=//p" "$vendor/SOURCE"; }
ffmpeg="$(value ffmpeg)"
emsdk="$(value emsdk)"
signing_key="$(value ffmpeg_signing_key)"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
tarball="$work/ffmpeg-$ffmpeg.tar.xz"
curl -fsSL -o "$tarball" "https://ffmpeg.org/releases/ffmpeg-$ffmpeg.tar.xz"
curl -fsSL -o "$tarball.asc" "https://ffmpeg.org/releases/ffmpeg-$ffmpeg.tar.xz.asc"
gnupg="$work/gnupg"
mkdir -m 700 "$gnupg"
curl -fsSL https://ffmpeg.org/ffmpeg-devel.asc | GNUPGHOME="$gnupg" gpg --quiet --import
# 取り込んだ公開鍵が SOURCE の公開鍵と同じで、その公開鍵で署名されていることを検証する。
GNUPGHOME="$gnupg" gpg --quiet --status-fd 1 --verify "$tarball.asc" "$tarball" 2>/dev/null |
  grep -q "^\[GNUPG:\] VALIDSIG $signing_key " || {
  echo "the signature of ffmpeg-$ffmpeg.tar.xz is not by $signing_key" >&2
  exit 1
}
tar -C "$work" -xf "$tarball"
cp "$vendor/glue.c" "$work/glue.c"

# 組む場所のパスが成果物に残っても同じになるよう、コンテナの中では、いつも /w に置く。
docker run --rm -u "$(id -u):$(id -g)" -v "$work:/w" -e "FFMPEG_VERSION=$ffmpeg" \
  "docker.io/emscripten/emsdk:$emsdk" bash -euo pipefail -c '
    cflags="-O3 -msimd128 -flto"
    mkdir -p /w/obj /w/out
    cd /w/obj
    emconfigure "/w/ffmpeg-$FFMPEG_VERSION/configure" \
      --target-os=none --arch=x86_32 --enable-cross-compile \
      --cc=emcc --cxx=em++ --ar=emar --ranlib=emranlib --nm=emnm \
      --disable-asm --disable-inline-asm --disable-x86asm --disable-pthreads --disable-w32threads --disable-os2threads \
      --disable-programs --disable-doc --disable-network --disable-autodetect --disable-debug --disable-runtime-cpudetect \
      --disable-everything --disable-avdevice --disable-avformat --disable-avfilter --disable-swscale --disable-swresample \
      --enable-decoder=mpeg2video,aac --enable-parser=mpegvideo \
      --optflags="$cflags" --extra-cflags="$cflags" --extra-ldflags="$cflags" >/w/out/configure.log ||
      { cat /w/obj/ffbuild/config.log >&2; exit 1; }
    # 長いので、落ちたときだけ出す。
    emmake make -j"$(nproc)" >/w/out/make.log 2>&1 || { cat /w/out/make.log >&2; exit 1; }
    # shellcheck disable=SC2086 # cflags は空白で分けて渡す
    emcc $cflags -I. -I"/w/ffmpeg-$FFMPEG_VERSION" /w/glue.c \
      libavcodec/libavcodec.a libavutil/libavutil.a \
      -sMODULARIZE=1 -sEXPORT_ES6=1 -sENVIRONMENT=worker \
      -sALLOW_MEMORY_GROWTH=1 -sINITIAL_MEMORY=64MB -sMAXIMUM_MEMORY=512MB -sFILESYSTEM=0 \
      -sEXPORTED_RUNTIME_METHODS=HEAPU8,HEAP32,HEAPF32 -sEXPORTED_FUNCTIONS=_malloc,_free \
      -o /w/out/decoder.js
  '

mkdir -p "$vendor/decoder/dist" "$public"
cp "$work/out/decoder.js" "$vendor/decoder/dist/decoder.js"
cp "$work/out/decoder.wasm" "$public/decoder.wasm"
# 組んだ環境によらず、同じ権限で置く。
chmod 644 "$public/decoder.wasm"
cp "$work/ffmpeg-$ffmpeg/COPYING.LGPLv2.1" "$public/COPYING.LGPLv2.1"
# /about の同梱物の一覧 (設計書 付録 B) が、パッケージのライセンスの文書として拾う。
cp "$work/ffmpeg-$ffmpeg/COPYING.LGPLv2.1" "$vendor/decoder/COPYING.LGPLv2.1"
cat >"$public/SOURCE" <<EOF
FFmpeg $ffmpeg (LGPL-2.1-or-later), configured with only the mpeg2video and aac decoders and the mpegvideo parser.
Source: https://ffmpeg.org/releases/ffmpeg-$ffmpeg.tar.xz
Glue code and build script: frontend/vendor/ffmpeg/glue.c and frontend/scripts/build-ffmpeg.sh in https://github.com/SlashNephy/kotatsu
To replace this library, rebuild decoder.wasm (next to this file) together with decoder.js using the build script above, then rebuild the frontend that bundles decoder.js.
EOF
