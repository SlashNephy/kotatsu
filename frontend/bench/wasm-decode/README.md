# WASM デコードの測定

ブラウザ内で MPEG-2 をそのままデコードする方式（#1516）を、mpeg2toh264 の変換（設計書 7.2）と比較して計測する道具です。
配信する画面には入りません。計測した結果は #1516 にコメントで残しています。

測れるもの:

- 同じ worker 1 本での速さ（コマ/秒）と、1 コマのデコード時間の分位
- mpeg2toh264 の出力のビットレートと、輝度の PSNR
- パケットを落とした TS で、それぞれが出せるコマの数
- WASM の組み方（最適化の段階、SIMD、LTO）による速さと大きさ

描画（WebGL2）と音の時計は、まだ入っていません。WASM の方式は、デコードまでを計測します。

## 要るもの

- mise（FFmpeg と Node.js を入れます）
- Docker（WASM を組む emscripten のイメージを動かします）
- gpg と curl（FFmpeg の tarball の署名を確認します）

作ったものは、どれも `.work/` に置きます（git には入りません）。

## 使い方

FFmpeg を入れます。

```bash
MISE_ENV=wasm-decode-bench mise -C frontend install
```

WASM の FFmpeg を組みます。`../../vendor/ffmpeg/SOURCE` のバージョンの tarball を取り、署名を確認してから、組み方ごとに `.work/out/<名前>/` に組みます。
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

測定のページを配信します。

```bash
mise run //frontend:wasm-decode:serve
```

`http://127.0.0.1:18516/` を開き、ボタンで計測します。結果は 1 行に 1 つの JSON で出ます。
実機から開くときは、ポートと待ち受けるアドレスを渡します。LAN に開くので、計測し終えたら止めてください。

```bash
mise run //frontend:wasm-decode:serve 18516 0.0.0.0
```

結果をファイルにコピーしたら、映像と方式ごとにまとめられます。

```bash
node frontend/bench/wasm-decode/summarize.mjs results.jsonl
```

ブラウザの自動操作で計測するときは、ページの `window.runDecode(映像, 組み方)` と `window.runConvert(映像, { save })` を呼びます。
どちらも結果のオブジェクトを返します。

## デコードと GPU へのアップロードを分けて計測する

#2162 の切り分けのために、製品の経路と同じく、デコードしたコマを WebGL2 の R8 のテクスチャ 3 枚へ上げて計測できます。
WASM は組み方の名前 `product` で製品のもの（`vendor/ffmpeg/decoder` と `public/ffmpeg/decoder.wasm`）を使うので、組まなくても計測できます。

ページの「切り分けを計測する」を押すと、`mandel_1440.ts` と `testsrc2_1440.ts` で、アップロードなしと次の 3 通りの上げ方を、交互に 2 回ずつ計測します。
1 回は 300 コマで打ち切ります。打ち切った回は音をデコードせず、`fps` は出しません（`null`）。映像だけの速さは `videoFps` で見ます。

| 上げ方     | 渡すもの                                     |
| ---------- | -------------------------------------------- |
| `offset`   | WASM のメモリ全体と面の位置（製品と同じ）    |
| `subarray` | 面の範囲だけの `subarray`（複製しない）      |
| `copy`     | 面の範囲を JS の `Uint8Array` に複製したもの |

結果には PES ごとの時間（`avgMs`、`p50`、`p95`）に加えて、アップロードの時間（`uploadAvgMs` など）、`gl.flush` の時間（`flushAvgMs` など）、
アップロードを差し引いたデコードの時間（`decodeOnlyAvgMs` など）、WebGL の renderer、worker の `performance.now()` の粒度（`timer`）が入ります。
粒度が粗い端末では、1 ms 未満の区間の分位が 0 か粒度の値になるので、平均と一緒に読みます。

ブラウザの自動操作で計測するときは、`window.runDecode(映像, 'product', false, { upload: 'offset', maxFrames: 300 })` を呼びます。

## mpeg2toh264 の出力を確認する

`runConvert` に `{ save: true }` を渡すと、出力の MP4 を `.work/save/<映像>.mp4` に書きます。

輝度の PSNR は、元の TS と比較して出します。

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

| ファイル                            | 内容                                                                                                     |
| ----------------------------------- | -------------------------------------------------------------------------------------------------------- |
| `build.sh`、`build-in-container.sh` | FFmpeg を mpeg2video・AAC・Opus のデコーダと mpegvideo の parser、swresample だけに絞り、WASM に組みます |
| `glue.c`                            | WASM の中のデコーダを JS から呼ぶ関数                                                                    |
| `make-media.sh`、`drop-packets.mjs` | 合成の映像と、パケットを落とした TS を作ります                                                           |
| `serve.mjs`                         | 測定のページ、WASM、映像、mpeg2toh264 を配信し、mpeg2toh264 の出力を受け取ります                         |
| `page/`                             | 測定のページと、デコードの worker、TS を PES に分ける処理                                                |
| `summarize.mjs`、`psnr.mjs`         | 結果をまとめ、PSNR を出します                                                                            |
