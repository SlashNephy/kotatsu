# mpeg2toh264

ブラウザ内での変換（設計書7.2）に使う、[otya128/mpeg2toh264](https://github.com/otya128/mpeg2toh264) のプレイヤーです。
npmに公開されていないので、`SOURCE`で固定したcommitのソースからビルドしたものを置いています。
ここのファイルは手で編集せず、次のコマンドでビルドし直してください。

```bash
MISE_ENV=mpeg2toh264 mise run //frontend:mpeg2toh264
```

ビルドには、`mise.mpeg2toh264.toml`で固定したRustとwasm-bindgenを使います。
同じcommitからは同じファイルができるので、CIは、ビルドし直したものとここのファイルに差分がないことを確かめます。

| ディレクトリ | パッケージ | ライセンス |
| --- | --- | --- |
| `player/` | `@mpeg2toh264/player`（変換のワーカーを含む） | MIT |
| `yadif/` | `@mpeg2toh264/yadif`（FFmpegのyadifに由来するWebGLのデインターレーサー） | LGPL-2.1-or-later |
| `../../public/mpeg2toh264/` | 変換に使うWebAssembly | MIT |

`@mpeg2toh264/yadif`は、LGPLの条件に沿って、ほかのコードと混ぜずに独立したモジュールのまま配信します。
ソースの入手先は、上流のリポジトリの`packages/yadif`です。
差し替えるときは、このディレクトリの`yadif/dist`を、同じAPIを持つビルドに置き換えてください。
