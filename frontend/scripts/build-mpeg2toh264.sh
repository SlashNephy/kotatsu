#!/usr/bin/env bash
# mpeg2toh264 のプレイヤーを、frontend/vendor/mpeg2toh264/SOURCE で固定した commit からビルドし、frontend に置く (設計書 7.2)。
#
# mpeg2toh264 のプレイヤーは npm に公開されていないので、ソースからビルドしたものをリポジトリに置く。
# ビルドには、mise で固定した Rust と wasm-bindgen と Node.js を使う。
# 同じ commit からは同じものができるよう、ビルドしたディレクトリの絶対パスを成果物に残さない。
#
# 置くもの:
#   frontend/vendor/mpeg2toh264/player  @mpeg2toh264/player (MIT)。変換のワーカーを含む
#   frontend/vendor/mpeg2toh264/yadif   @mpeg2toh264/yadif (LGPL-2.1-or-later)。差し替えられるよう、別のパッケージのままにする
#   frontend/public/mpeg2toh264/        変換に使う WebAssembly と、yadif のライセンス文書とソースの入手先。明示的な URL で配信する
set -euo pipefail

frontend="$(cd "$(dirname "$0")/.." && pwd)"
vendor="$frontend/vendor/mpeg2toh264"
public="$frontend/public/mpeg2toh264"
# shellcheck source=/dev/null
source <(grep -E '^(repository|commit)=' "$vendor/SOURCE")

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
src="$work/mpeg2toh264"
git init -q "$src"
git -C "$src" fetch -q --depth 1 "$repository" "$commit"
git -C "$src" -c advice.detachedHead=false checkout -q FETCH_HEAD

# wasm-bindgen は、成果物に自分の版 (ビルドした commit を含む) を書くので、mise で固定した配布物を使う。
# cargo install で入れたものが ~/.cargo/bin にあると、そちらが先に見つかってしまう。
if command -v mise >/dev/null 2>&1; then
  PATH="$(mise where github:wasm-bindgen/wasm-bindgen):$PATH"
fi

# Rust は、パニックのメッセージなどにソースの絶対パスを埋め込むので、どこでビルドしても同じパスにする。
# RUSTFLAGS で渡すと、上流の .cargo/config.toml の rustflags (simd128 など) が使われなくなるので、
# 親のディレクトリの設定に書き、上流の設定と合わせて使わせる。
cargo_home="${CARGO_HOME:-$HOME/.cargo}"
mkdir -p "$work/.cargo"
cat >"$work/.cargo/config.toml" <<EOF
[target.wasm32-unknown-unknown]
rustflags = ["--remap-path-prefix=$src=/mpeg2toh264", "--remap-path-prefix=$cargo_home=/cargo"]
EOF
unset RUSTFLAGS CARGO_ENCODED_RUSTFLAGS
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
  # GNU と BSD のどちらの install と sed でも動くよう、-D と -i を使わない。
  (cd "$from" && find dist -type f ! -name '*.map') | while IFS= read -r file; do
    mkdir -p "$to/$(dirname "$file")"
    grep -v '^//# sourceMappingURL=' "$from/$file" >"$to/$file" || true
  done
  cp "$from/LICENSE" "$to/LICENSE"
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
mkdir -p "$public"
cp "$src/packages/player/wasm/mpeg2toh264_wasm_bg.wasm" "$public/mpeg2toh264_wasm_bg.wasm"
cp "$src/LICENSE" "$public/LICENSE"
# yadif は LGPL なので、配信する SPA に、ライセンス文書とソースの入手先を含める (付録 B)。
cp "$src/packages/yadif/LICENSE" "$public/yadif-LICENSE"
cat >"$public/yadif-SOURCE" <<EOF
@mpeg2toh264/yadif (LGPL-2.1-or-later) is built from the following source.
Its license is in yadif-LICENSE next to this file.

repository: $repository
commit: $commit
directory: packages/yadif
EOF

echo "built mpeg2toh264 $commit"
