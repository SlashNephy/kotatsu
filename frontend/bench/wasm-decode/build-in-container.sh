#!/usr/bin/env bash
# emscripten のイメージの中で、FFmpeg をデコーダだけに絞って組み、glue.c とつないで dec.mjs と dec.wasm を作る。build.sh から呼ぶ。
# 使い方: build-in-container.sh <名前> <最適化 (-O2 など)> <SIMD 0/1> <LTO 0/1>
# 手書きアセンブリと pthreads は使わない (crossOriginIsolated が要らないようにする。#1516 の「落とし穴」)。
set -euo pipefail
name=$1 opt=$2 simd=$3 lto=$4
cflags="$opt"
if [ "$simd" = 1 ]; then cflags="$cflags -msimd128"; fi
if [ "$lto" = 1 ]; then cflags="$cflags -flto"; fi
src="/w/.work/src/ffmpeg-$FFMPEG_VERSION"
out="/w/.work/out/$name"
obj="/w/.work/obj/$name"
rm -rf "$obj" "$out"
mkdir -p "$obj" "$out"
cd "$obj"
emconfigure "$src/configure" \
  --target-os=none --arch=x86_32 --enable-cross-compile \
  --cc=emcc --cxx=em++ --ar=emar --ranlib=emranlib --nm=emnm \
  --disable-asm --disable-inline-asm --disable-x86asm --disable-pthreads --disable-w32threads --disable-os2threads \
  --disable-programs --disable-doc --disable-network --disable-autodetect --disable-debug --disable-runtime-cpudetect \
  --disable-everything --disable-avdevice --disable-avformat --disable-avfilter --disable-swscale --disable-postproc \
  --enable-decoder=mpeg2video,aac,opus --enable-parser=mpegvideo --enable-swresample \
  --optflags="$cflags" --extra-cflags="$cflags" --extra-ldflags="$cflags" >"$out/configure.log"
emmake make -j"$(nproc)" >"$out/make.log" 2>&1
# shellcheck disable=SC2086 # cflags は空白で分けて渡す
emcc $cflags -I. -I"$src" /w/glue.c \
  libavcodec/libavcodec.a libswresample/libswresample.a libavutil/libavutil.a \
  -sMODULARIZE=1 -sEXPORT_ES6=1 -sENVIRONMENT=worker -sALLOW_MEMORY_GROWTH=1 -sINITIAL_MEMORY=64MB \
  -sFILESYSTEM=0 -sEXPORTED_RUNTIME_METHODS=HEAPU8,HEAP32 -sEXPORTED_FUNCTIONS=_malloc,_free \
  -o "$out/dec.mjs"
