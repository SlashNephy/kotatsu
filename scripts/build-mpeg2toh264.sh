#!/usr/bin/env bash
# mpeg2toh264 のプレイヤーを、web/vendor/mpeg2toh264/SOURCE で固定した commit からビルドし、web に置く (設計書 7.2)。
#
# mpeg2toh264 のプレイヤーは npm に公開されていないので、ソースからビルドしたものをリポジトリに置く。
# ビルドには、mise で固定した Rust と wasm-bindgen と Node.js を使う。
# 同じ commit からは同じものができるよう、ビルドしたディレクトリの絶対パスを成果物に残さない。
#
# 置くもの:
#   web/vendor/mpeg2toh264/player  @mpeg2toh264/player (MIT)。変換のワーカーを含む
#   web/vendor/mpeg2toh264/yadif   @mpeg2toh264/yadif (LGPL-2.1-or-later)。差し替えられるよう、別のパッケージのままにする
#   web/public/mpeg2toh264/        変換に使う WebAssembly。明示的な URL で配信する
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
vendor="$root/web/vendor/mpeg2toh264"
public="$root/web/public/mpeg2toh264"
# shellcheck source=/dev/null
source <(grep -E '^(repository|commit)=' "$vendor/SOURCE")

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
src="$work/mpeg2toh264"
git init -q "$src"
git -C "$src" fetch -q --depth 1 "$repository" "$commit"
git -C "$src" -c advice.detachedHead=false checkout -q FETCH_HEAD

# Rust は、パニックのメッセージなどにソースの絶対パスを埋め込むので、どこでビルドしても同じパスにする。
cargo_home="${CARGO_HOME:-$HOME/.cargo}"
export RUSTFLAGS="--remap-path-prefix=$src=/mpeg2toh264 --remap-path-prefix=$cargo_home=/cargo"
(
  cd "$src"
  # 上流のスクリプトは、wasm-bindgen の CLI と crate の版が揃っていることを確かめてからビルドする。
  ./tools/build-wasm.sh >/dev/null
  npm ci --ignore-scripts --no-audit --no-fund >/dev/null
  npm run packages:build >/dev/null
)

# copy_package は、ビルドしたパッケージの dist を置き、npm のパッケージとして使えるようにする。
# ソースマップは、ビルドしたディレクトリのパスを含むので置かない。
copy_package() {
  local name="$1" license="$2"
  local from="$src/packages/$name" to="$vendor/$name"
  rm -rf "$to"
  mkdir -p "$to"
  (cd "$from" && find dist -type f ! -name '*.map' -print0 | xargs -0 -I{} install -D -m 0644 {} "$to/{}")
  find "$to/dist" -type f -name '*.js' -exec sed -i '/^\/\/# sourceMappingURL=/d' {} +
  find "$to/dist" -type f -name '*.d.ts' -exec sed -i '/^\/\/# sourceMappingURL=/d' {} +
  install -m 0644 "$from/LICENSE" "$to/LICENSE"
  cat >"$to/package.json" <<EOF
{
  "name": "@mpeg2toh264/$name",
  "version": "0.0.0",
  "private": true,
  "description": "$name of mpeg2toh264, built by scripts/build-mpeg2toh264.sh",
  "type": "module",
  "license": "$license",
  "exports": {
    ".": {
      "types": "./dist/index.d.ts",
      "import": "./dist/index.js"
    }
  }
}
EOF
}

copy_package player MIT
copy_package yadif LGPL-2.1-or-later
rm -rf "$public"
install -D -m 0644 "$src/packages/player/wasm/mpeg2toh264_wasm_bg.wasm" "$public/mpeg2toh264_wasm_bg.wasm"
install -m 0644 "$src/LICENSE" "$public/LICENSE"

echo "built mpeg2toh264 $commit"
