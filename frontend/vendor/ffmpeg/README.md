# FFmpeg のデコーダ

ブラウザ内でデコードする経路（設計書 7.2、#1516）で使う、WebAssembly の FFmpeg です。
FFmpeg を mpeg2video と AAC のデコーダ、mpegvideo の parser だけに絞り、`glue.c` とつないで emscripten で組んだものを置いています。
ここの `decoder/dist` と `../../public/ffmpeg/` は手で編集せず、次のコマンドで組み直してください。

```bash
mise run //frontend:ffmpeg
```

組むには Docker と curl と gpg が要ります。
FFmpeg のリリースの tarball を取り、`SOURCE` の鍵で署名を確かめてから、`SOURCE` で固定した emscripten のイメージの中で組みます。
同じ版からは同じファイルができるので、CI は、組み直したものとここのファイルに差分がないことを確かめます。

| ファイル | 中身 | ライセンス |
| --- | --- | --- |
| `SOURCE` | FFmpeg の版、emscripten のイメージ、FFmpeg の署名鍵。Renovate が追います | |
| `glue.c` | WASM の中のデコーダを JS から呼ぶ関数 | LGPL-2.1-or-later（FFmpeg とリンクするため） |
| `decoder/` | `@kotatsu/ffmpeg-decoder`。WebAssembly を読み込む JS（`dist/decoder.js`）と、その型 | LGPL-2.1-or-later |
| `../../public/ffmpeg/` | デコーダの WebAssembly と、ライセンス文書、ソースの入手先 | LGPL-2.1-or-later |

FFmpeg は、LGPL の条件に沿って、ほかのコードと混ぜずに独立したモジュールのまま配信します。
WebAssembly を読み込む `decoder/dist/decoder.js` は、WebAssembly の関数を縮めた名前で呼び、画面の JS に束ねられます。
そのため、差し替えるときは、`decoder.wasm` だけを置き換えず、上のコマンドで `decoder.wasm` と `decoder.js` を組で組み直してから、画面を組み直してください。
