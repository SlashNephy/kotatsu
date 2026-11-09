# mpeg2toh264

ブラウザ内での変換（設計書7.2）に使う、[otya128/mpeg2toh264](https://github.com/otya128/mpeg2toh264) のプレイヤーです。
npmに公開されていないので、`SOURCE`で固定したcommitのソースに`patches/`の修正を当て、ビルドしたものを置いています。
ここのファイルは手で編集せず、次のコマンドでビルドし直してください。

```bash
MISE_ENV=mpeg2toh264 mise run //frontend:mpeg2toh264
```

ビルドには、`mise.mpeg2toh264.toml`で固定したRustとwasm-bindgenを使います。
同じcommitと修正からは同じファイルができるので、CIは、ビルドし直したものとここのファイルに差分がないことを確かめます。

`patches/`は、上流にまだ入っていない修正です。
`SOURCE`のcommitを変えて修正が当たらなくなったら、そのcommitに合わせて作り直すか、上流に入っていれば消してください。

| ディレクトリ | パッケージ | ライセンス |
| --- | --- | --- |
| `player/` | `@mpeg2toh264/player`（変換のワーカーを含む） | MIT |
| `yadif/` | `@mpeg2toh264/yadif`（FFmpegのyadifに由来するWebGLのデインターレーサー） | LGPL-2.1-or-later |
| `../../public/mpeg2toh264/` | 変換に使うWebAssembly | MIT |

`@mpeg2toh264/yadif`は、LGPLの条件に沿って、ほかのコードと混ぜずに独立したモジュールのまま配信します。
ソースの入手先は、上流のリポジトリの`packages/yadif`と、`patches/`（配信するファイルの隣にも置きます）です。
差し替えるときは、このディレクトリの`yadif/dist`を、同じAPIを持つビルドに置き換えてください。
