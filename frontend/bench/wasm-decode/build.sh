#!/usr/bin/env bash
# SOURCE で固定した版の FFmpeg から、mpeg2video・AAC・Opus のデコーダだけを emscripten で WASM に組む (#1516)。
#
# 使い方: build.sh [組み方の名前...]
# 名前を省くと、VARIANTS の組み方を全部組む。組んだものは .work/out/<名前>/ に置き、大きさと組む時間を .work/out/sizes.tsv に書く。
# FFmpeg の tarball は、SOURCE の鍵で署名を確かめてから使う。組むのは Docker の emscripten のイメージの中で行う。
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
work="$here/.work"
value() { sed -n "s/^$1=//p" "$here/SOURCE"; }
ffmpeg="$(value ffmpeg)"
emsdk="$(value emsdk)"
ffmpeg_signing_key="$(value ffmpeg_signing_key)"
image="docker.io/emscripten/emsdk:$emsdk"

# 名前 最適化 SIMD LTO
VARIANTS=(
  "O3 -O3 0 0"
  "O3-simd -O3 1 0"
  "O3-lto -O3 0 1"
  "O3-simd-lto -O3 1 1"
  "O2-simd-lto -O2 1 1"
  "Os-simd-lto -Os 1 1"
)

mkdir -p "$work/src" "$work/out"
tarball="$work/src/ffmpeg-$ffmpeg.tar.xz"
if [ ! -d "$work/src/ffmpeg-$ffmpeg" ]; then
  curl -fsSL -o "$tarball" "https://ffmpeg.org/releases/ffmpeg-$ffmpeg.tar.xz"
  curl -fsSL -o "$tarball.asc" "https://ffmpeg.org/releases/ffmpeg-$ffmpeg.tar.xz.asc"
  gnupg="$(mktemp -d)"
  trap 'rm -rf "$gnupg"' EXIT
  curl -fsSL https://ffmpeg.org/ffmpeg-devel.asc | GNUPGHOME="$gnupg" gpg --quiet --import
  # 取り込んだ鍵が SOURCE の鍵と同じで、その鍵で署名されていることを確かめる。
  GNUPGHOME="$gnupg" gpg --quiet --status-fd 1 --verify "$tarball.asc" "$tarball" 2>/dev/null |
    grep -q "^\[GNUPG:\] VALIDSIG $ffmpeg_signing_key " || {
    echo "the signature of ffmpeg-$ffmpeg.tar.xz is not by $ffmpeg_signing_key" >&2
    exit 1
  }
  tar -C "$work/src" -xf "$tarball"
fi

selected=("$@")
for variant in "${VARIANTS[@]}"; do
  read -r name opt simd lto <<<"$variant"
  if [ "${#selected[@]}" -gt 0 ] && [[ ! " ${selected[*]} " == *" $name "* ]]; then
    continue
  fi
  start=$(date +%s)
  docker run --rm -u "$(id -u):$(id -g)" -v "$here:/w" -e "FFMPEG_VERSION=$ffmpeg" "$image" \
    bash /w/build-in-container.sh "$name" "$opt" "$simd" "$lto"
  end=$(date +%s)
  wasm="$work/out/$name/dec.wasm"
  printf '%s\t%d\t%d\t%d\t%d\n' "$name" "$((end - start))" "$(stat -c %s "$wasm")" \
    "$(gzip -9c "$wasm" | wc -c)" "$(node -e 'process.stdout.write(String(require("node:zlib").brotliCompressSync(require("node:fs").readFileSync(process.argv[1])).length))' "$wasm")" |
    tee -a "$work/out/sizes.tsv"
done
echo "name build_seconds wasm_bytes gzip_bytes brotli_bytes の順に .work/out/sizes.tsv に書きました"
