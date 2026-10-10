# Kotatsu

Kotatsu は、日本のテレビ放送を予約、録画、視聴するための Web アプリケーションです。
チューナーサーバーには [Mirakurun](https://github.com/Chinachu/Mirakurun)、[mirakc](https://github.com/mirakc/mirakc)、Mahiron を使います。
[EPGStation](https://github.com/l3tnun/EPGStation) の後継として、録画予約から再生までを一つのアプリケーションで扱います。

名前は、こたつに入ってテレビや録画をゆったり楽しむ情景から取りました。

> [!WARNING]
> Kotatsu は開発中で、まだ録画には使えません。
> 番組表の同期、ルール予約、チューナーの割り当て、ログインは動きますが、録画の実行、再生、通知はこれから作ります。
> 進み具合は、[v1 のマイルストーン](https://github.com/SlashNephy/kotatsu/milestone/1)で確かめられます。

## 特徴

- **1 つのアプリケーションで完結します。** 番組表、ルール予約、録画、録画の再生、ライブ視聴を、Mirakurun 系のチューナーサーバーの上で扱います。
- **1 台から Kubernetes まで、同じバイナリで動きます。** 役割ごとのロールを、1 プロセスにまとめても、別々のレプリカに分けても動かせます。録画の割り振りや後処理の状態は、PostgreSQL に持ちます。
- **何も設定しなくても安全です。** ログインは必須で、パスワードでのログインには第二要素 (TOTP) も必須です。パスキーと OIDC でもログインできます。既定では、プライベートネットワークからしか接続できません。
- **ブラウザでそのまま見られます。** 録画は .m2ts のまま保存し、再生するときにブラウザ内で変換します。ニコニコ実況のコメント、ARIB 字幕、データ放送にも対応する予定です。
- **EPGStation から移行できます。** `kotatsu import epgstation` で、ルール、予約、録画済みの番組とファイル、録画履歴を取り込めるようにします。

## 構成

単一のバイナリ `kotatsu` のサブコマンドで、ロールを選びます。`all` を指定すると、全ロールが 1 つのプロセスで動きます。

| ロール | 役割名 | 担当 |
| --- | --- | --- |
| tenban (天板) | server | GraphQL API、画面の配信、ライブ視聴と録画の配信 |
| neko (猫) | scheduler | 番組表の同期、ルールの照合、チューナーの割り当て。アクティブなインスタンスは 1 つ |
| mikan (みかん) | recorder | チューナーサーバーから受け取った TS を、ファイルに書き込む |
| nekonote (猫の手) | worker | サムネイル、容量の管理、通知などの後処理 |
| kyusu (急須) | transcoder | サーバー側での、その場のトランスコード (任意) |

詳しい設計は、[設計書](docs/spec.v1.md)にまとめています。

## 動かす

Kotatsu は、PostgreSQL と、チューナーサーバー (Mirakurun、mirakc、Mahiron のどれか) を使います。PostgreSQL は、CI では 17 で確かめています。
コンテナイメージは、`ghcr.io/slashnephy/kotatsu` で配布しています。

| タグ | 内容 |
| --- | --- |
| `edge` | main の最新。FFmpeg を含まない |
| `edge-ffmpeg` | main の最新。サーバー側のトランスコードに使う FFmpeg を含む |

### 設定

設定は、YAML のファイルか、`KOTATSU_` で始まる環境変数で渡します。主な項目は次のとおりです。

| 環境変数 | 内容 |
| --- | --- |
| `KOTATSU_DATABASE_URL` | PostgreSQL の接続先 (`postgres://user:password@host:5432/kotatsu`) |
| `KOTATSU_AUTH_SECRET_KEY` | アプリの秘密キー。32 バイト以上のランダムな文字列 (`openssl rand -base64 48` など)。TOTP の秘密キーなどの暗号化に使い、変えると読めなくなります |
| `KOTATSU_TUNER_URL` | チューナーサーバーの URL (`http://mirakurun:40772`) |
| `KOTATSU_STORAGE_POOLS` | 録画の保存先。`main=/mnt/main,sub=/mnt/sub` のように「ID=パス」をカンマで区切る |
| `KOTATSU_SERVER_LISTEN` | 待ち受けアドレス (既定は `:8080`) |
| `KOTATSU_SERVER_ALLOWED_NETWORKS` | 接続を受け付けるネットワーク。既定はプライベートアドレスだけ |

秘密情報は、名前の末尾に `_FILE` を付けた環境変数で、ファイルのパスとしても渡せます (`KOTATSU_AUTH_SECRET_KEY_FILE` など)。

### 起動と初回セットアップ

1. DB のスキーマを作ります。

   ```bash
   kotatsu migrate
   ```

2. 全ロールを 1 つのプロセスで起動します。

   ```bash
   kotatsu all
   ```

3. 管理者がまだいなければ、起動時のログにセットアップトークンが出ます。ブラウザで `http://<ホスト>:8080/setup` を開き、トークンを入れて最初の管理者を作ります。
4. 作った管理者でログインし、認証アプリ (TOTP) を登録します。

## 開発

ツールチェーンは [mise](https://mise.jdx.dev/) で入れます。

```bash
mise install
```

| コマンド | 内容 |
| --- | --- |
| `mise run generate` | sqlc、gqlgen、GraphQL のスキーマ、マイグレーションの検証などの生成物を作り直す |
| `mise run test` | Go のテストを `-race` で流す |
| `mise run lint` | golangci-lint を流す |
| `mise run web:check` | 画面の lint と型を確かめる |
| `mise run web:test` | 画面のテストを流す |
| `mise run db:diff -- <名前>` | `db/schema.sql` の変更から、マイグレーションを作る |

DB を使うテストは、`KOTATSU_TEST_DATABASE_URL` に PostgreSQL の接続先を渡したときだけ流れます。

## ライセンス

Kotatsu は [Apache License 2.0](LICENSE) で公開しています。
同梱するソフトウェアとデータのライセンスは、[設計書の付録 B](docs/spec.v1.md#付録-b-ライセンスの境界) にまとめています。
