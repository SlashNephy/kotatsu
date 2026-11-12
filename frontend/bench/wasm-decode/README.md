# WASM デコードの測定

ブラウザ内で MPEG-2 をそのままデコードする方式（#1516）を、mpeg2toh264 の変換（設計書 7.2）と比べて測る道具です。
配信する画面には入りません。測った結果は #1516 にコメントで残しています。

測れるもの:

- 同じ worker 1 本での速さ（コマ/秒）と、1 コマのデコード時間の分位
- mpeg2toh264 の出力のビットレートと、輝度の PSNR
- パケットを落とした TS で、それぞれが出せるコマの数
- WASM の組み方（最適化の段階、SIMD、LTO）による速さと大きさ

描画（WebGL2）と音の時計は、まだ入っていません。WASM の方式は、デコードまでを測ります。

## 要るもの

- mise（FFmpeg と Node.js を入れます）
- Docker（WASM を組む emscripten のイメージを動かします）
- gpg と curl（FFmpeg の tarball の署名を確かめます）

作ったものは、どれも `.work/` に置きます（git には入りません）。

## 使い方

FFmpeg を入れます。

```bash
MISE_ENV=wasm-decode-bench mise -C frontend install
```

WASM の FFmpeg を組みます。`SOURCE` の版の tarball を取り、署名を確かめてから、組み方ごとに `.work/out/<名前>/` に組みます。
大きさと組む時間は `.work/out/sizes.tsv` に書きます。組み方の名前を渡すと、その組み方だけを組みます。

```bash
MISE_ENV=wasm-decode-bench mise run //frontend:wasm-decode:build
```

```bash
MISE_ENV=wasm-decode-bench mise run //frontend:wasm-decode:build O3-simd-lto
```

合成の映像を作ります。絵柄 3 種類 × 幅 2 種類の 30 秒の 1080i と、パケットを落とした TS を `.work/media/` に作ります。

```bash
MISE_ENV=wasm-decode-bench mise run //frontend:wasm-decode:media
```

測定のページを配ります。

```bash
mise run //frontend:wasm-decode:serve
```

`http://127.0.0.1:18516/` を開き、ボタンで測ります。結果は 1 行に 1 つの JSON で出ます。
実機から開くときは、ポートと待ち受けるアドレスを渡します。LAN に開くので、測り終えたら止めてください。

```bash
mise run //frontend:wasm-decode:serve 18516 0.0.0.0
```

結果をファイルに写したら、映像と方式ごとにまとめられます。

```bash
node frontend/bench/wasm-decode/summarize.mjs results.jsonl
```

ブラウザの自動操作で測るときは、ページの `window.runDecode(映像, 組み方)` と `window.runConvert(映像, { save })` を呼びます。
どちらも結果のオブジェクトを返します。

## mpeg2toh264 の出力を調べる

`runConvert` に `{ save: true }` を渡すと、出力の MP4 を `.work/save/<映像>.mp4` に書きます。

輝度の PSNR は、元の TS と比べて出します。

```bash
MISE_ENV=wasm-decode-bench mise exec -C frontend -- node bench/wasm-decode/psnr.mjs bench/wasm-decode/.work/save/testsrc2_1440.ts.mp4 bench/wasm-decode/.work/media/testsrc2_1440.ts
```

出力には同じ絵のコマが重なって入り、PTS も重なります。そのため、PTS では元のコマと合わせられません。
`psnr.mjs` は、出力を 0〜12 コマずらして並べ、元のコマごとに最も近い値を取ります。

出せたコマの数は、違う絵の枚数で数えます。重なったコマを数えないためです。

```bash
MISE_ENV=wasm-decode-bench mise exec -C frontend -- sh -c 'ffmpeg -v error -i bench/wasm-decode/.work/save/loss_0.001.ts.mp4 -map 0:v -fps_mode passthrough -f framemd5 - | grep -v "^#" | cut -d, -f6 | sort -u | wc -l'
```

## ファイル

| ファイル                            | 中身                                                                                                     |
| ----------------------------------- | -------------------------------------------------------------------------------------------------------- |
| `SOURCE`                            | WASM に組む FFmpeg の版、emscripten のイメージ、FFmpeg の署名鍵。Renovate が追います                     |
| `build.sh`、`build-in-container.sh` | FFmpeg を mpeg2video・AAC・Opus のデコーダと mpegvideo の parser、swresample だけに絞り、WASM に組みます |
| `glue.c`                            | WASM の中のデコーダを JS から呼ぶ関数                                                                    |
| `make-media.sh`、`drop-packets.mjs` | 合成の映像と、パケットを落とした TS を作ります                                                           |
| `serve.mjs`                         | 測定のページ、WASM、映像、mpeg2toh264 を配り、mpeg2toh264 の出力を受け取ります                           |
| `page/`                             | 測定のページと、デコードの worker、TS を PES に分ける処理                                                |
| `summarize.mjs`、`psnr.mjs`         | 結果をまとめ、PSNR を出します                                                                            |
