#!/usr/bin/env bash
# mpeg2toh264 のプレイヤーを、frontend/vendor/<名前>/SOURCE で固定した commit からビルドし、frontend に置く (設計書 7.2)。
# 名前は引数で渡す。既定は mpeg2toh264 (otya128/mpeg2toh264)。評価のために並べるフォークは mpeg2toh264-fork (#2287)。
# パッケージは @<名前>/player と @<名前>/yadif の名前で置く。同じ名前の 2 つのパッケージは並べられないため。
#
# mpeg2toh264 のプレイヤーは npm に公開されていないので、ソースからビルドしたものをリポジトリに置く。
# ビルドには、mise で固定した Rust と wasm-bindgen と Node.js を使う。
# 同じ commit からは同じものができるよう、ビルドしたディレクトリの絶対パスを成果物に残さない。
#
# 置くもの:
#   frontend/vendor/<名前>/player  @<名前>/player (MIT)。変換のワーカーを含む
#   frontend/vendor/<名前>/yadif   @<名前>/yadif (LGPL-2.1-or-later)。差し替えられるよう、別のパッケージのままにする
#   frontend/public/<名前>/        変換に使う WebAssembly と、yadif のライセンス文書とソースの入手先。明示的な URL で配信する
set -euo pipefail
# 修正がすべて上流に入り、patches/ が空になっても動くようにする。
shopt -s nullglob

frontend="$(cd "$(dirname "$0")/.." && pwd)"
name="${1:-mpeg2toh264}"
vendor="$frontend/vendor/$name"
public="$frontend/public/$name"
# shellcheck source=/dev/null
source <(grep -E '^(repository|commit)=' "$vendor/SOURCE")

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
src="$work/mpeg2toh264"
git init -q "$src"
git -C "$src" fetch -q --depth 1 "$repository" "$commit"
git -C "$src" -c advice.detachedHead=false checkout -q FETCH_HEAD

# 上流に入っていない修正を、ファイル名の順に当てる。当たらなければ、SOURCE の commit に合わせて直す。
for patch in "$vendor"/patches/*.patch; do
  git -C "$src" apply "$patch"
done

# wasm-bindgen は、成果物に自分のバージョン (ビルドした commit を含む) を書くので、mise で固定した配布物を使う。
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
  # 上流のスクリプトは、wasm-bindgen の CLI と crate のバージョンが揃っていることを確認してからビルドする。
  ./tools/build-wasm.sh >/dev/null
  npm ci --ignore-scripts --no-audit --no-fund >/dev/null
  npm run packages:build >/dev/null
)

# copy_package は、ビルドしたパッケージの dist を置き、npm のパッケージとして使えるようにする。
# ソースマップは、ビルドしたディレクトリのパスを含むので置かない。
copy_package() {
  local package="$1" license="$2"
  local from="$src/packages/$package" to="$vendor/$package"
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
  "name": "@$name/$package",
  "version": "0.0.0",
  "private": true,
  "description": "$package of $name, built by scripts/build-mpeg2toh264.sh",
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
# 上流に変更を加えて配信するので、当てた修正も、ソースの一部として一緒に置く。
for patch in "$vendor"/patches/*.patch; do
  cp "$patch" "$public/"
done
cat >"$public/yadif-SOURCE" <<EOF
@$name/yadif (LGPL-2.1-or-later) is built from the following source,
with the .patch files next to this file applied in name order.
Its license is in yadif-LICENSE next to this file.

repository: $repository
commit: $commit
directory: packages/yadif
EOF

echo "built $name $commit"
