# mpeg2toh264（フォーク）

評価のために、[otya128/mpeg2toh264](https://github.com/otya128/mpeg2toh264)（`../mpeg2toh264`）と並べて置く、[soraneoumi/mpeg2toh264](https://github.com/soraneoumi/mpeg2toh264) のプレイヤーです（#2287）。
フォークはH.264への変換を持たず、ブラウザの中でMPEG-2とAACをデコードして再生します。画質のメニューでフォークを選んだときだけ使います。
npmに公開されていないので、`SOURCE`で固定したcommitのソースに`patches/`の修正を当て、ビルドしたものを置いています。
ここのファイルは手で編集せず、次のコマンドでビルドし直してください。

```bash
MISE_ENV=mpeg2toh264 mise run //frontend:mpeg2toh264-fork
```

ビルドには、otya128/mpeg2toh264と同じく、`mise.mpeg2toh264.toml`で固定したRustとwasm-bindgenを使います。
同じcommitと修正からは同じファイルができるので、CIは、ビルドし直したものとここのファイルに差分がないことを確認します。

`patches/`は、上流にまだ入っていない修正です。
`SOURCE`のcommitを変えて修正が当たらなくなったら、そのcommitに合わせて作り直すか、上流に入っていれば消してください。

| ディレクトリ | パッケージ | ライセンス |
| --- | --- | --- |
| `player/` | `@mpeg2toh264-fork/player`（デコードのワーカーを含む） | MIT |
| `yadif/` | `@mpeg2toh264-fork/yadif`（FFmpegのyadifに由来するWebGLのデインターレーサー） | LGPL-2.1-or-later |
| `../../public/mpeg2toh264-fork/` | デコードに使うWebAssembly | MIT |

otya128/mpeg2toh264と同じ名前のパッケージを並べられないので、`@mpeg2toh264-fork`の名前で置きます。
`@mpeg2toh264-fork/yadif`は、LGPLの条件に沿って、ほかのコードと混ぜずに独立したモジュールのまま配信します。
ソースの入手先は、上流のリポジトリの`packages/yadif`と、`patches/`（配信するファイルの隣にも置きます）です。
差し替えるときは、このディレクトリの`yadif/dist`を、同じAPIを持つビルドに置き換えてください。
