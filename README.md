# Kotatsu

Kotatsuは、日本のテレビ放送を予約、録画、視聴するためのWebアプリケーションです。
チューナーサーバーには[Mirakurun](https://github.com/Chinachu/Mirakurun)、[mirakc](https://github.com/mirakc/mirakc)、Mahironを使います。
[EPGStation](https://github.com/l3tnun/EPGStation)の後継として、録画予約から再生までを1つのアプリケーションで扱う設計です。

名前は、こたつに入ってテレビや録画をゆったり楽しむ情景から取りました。

> [!WARNING]
> Kotatsuは開発中で、まだ録画には使えません。
> いま動くのは、番組表の同期、ルール予約、チューナーの割り当て、ログインです。
> 録画の実行、再生、通知はこれから作ります。
> 進み具合は[v1のマイルストーン](https://github.com/SlashNephy/kotatsu/milestone/1)で確認できます。

## 特徴

番組表、ルール予約、録画、録画の再生、ライブ視聴を、Mirakurun系のチューナーサーバーの上で1つのアプリケーションにまとめます。

1台のマシンからKubernetesのマルチレプリカ構成まで、同じバイナリで動かせます。
役割ごとのロールは、1つのプロセスにまとめても、別々のレプリカに分けても構いません。
録画の割り振りや後処理の状態はPostgreSQLに保存するので、どのレプリカからも同じ状態を参照できます。

何も設定しない状態でも、安全に動くようにしています。
ログインは必須です。パスワードでログインするときは、認証アプリ（TOTP）の確認も求めます。
パスキーとOIDCでもログインできます。
接続を受け付けるのは、既定ではプライベートネットワークからだけです。

録画は.m2tsのまま保存し、再生するときにブラウザの中で変換します。
ニコニコ実況のコメント、ARIB字幕、データ放送にも対応する予定です。

EPGStationからは`kotatsu import epgstation`で移行できるようにします。
ルール、予約、録画済みの番組とファイル、録画履歴が取り込みの対象です。

## 構成

単一のバイナリ`kotatsu`のサブコマンドでロールを選びます。
`all`を指定すると、全ロールが1つのプロセスで動きます。

| ロール | 役割名 | 担当 |
| --- | --- | --- |
| tenban（天板） | server | GraphQL API、画面の配信、ライブ視聴と録画の配信 |
| neko（猫） | scheduler | 番組表の同期、ルールの照合、チューナーの割り当て。アクティブなインスタンスは1つ |
| mikan（みかん） | recorder | チューナーサーバーから受け取ったTSのファイルへの書き込み |
| nekonote（猫の手） | worker | サムネイル、容量の管理、通知などの後処理 |
| kyusu（急須） | transcoder | サーバー側でのその場のトランスコード。使うかどうかは任意 |

詳しい設計は[設計書](docs/spec.v1.md)にまとめました。

## 動かす

KotatsuにはPostgreSQLと、Mirakurun、mirakc、Mahironのいずれかのチューナーサーバーが必要です。
CIでは、PostgreSQL 17で動作を確認しています。

コンテナイメージは`ghcr.io/slashnephy/kotatsu`で配布しています。

| タグ | 内容 |
| --- | --- |
| `edge` | mainの最新版。FFmpegを含まない |
| `edge-ffmpeg` | mainの最新版。サーバー側のトランスコードに使うFFmpegを含む |

### 設定

設定はYAMLのファイルか、`KOTATSU_`で始まる環境変数で渡してください。
主な項目は次の表のとおりです。

| 環境変数 | 内容 |
| --- | --- |
| `KOTATSU_DATABASE_URL` | PostgreSQLの接続先（例: `postgres://user:password@host:5432/kotatsu`） |
| `KOTATSU_AUTH_SECRET_KEY` | アプリの秘密キー。32バイト以上のランダムな文字列を指定する（例: `openssl rand -base64 48`の出力）。TOTPの秘密キーなどの暗号化に使うので、後から変えると暗号化した値を読めなくなる |
| `KOTATSU_TUNER_URL` | チューナーサーバーのURL（例: `http://mirakurun:40772`） |
| `KOTATSU_STORAGE_POOLS` | 録画の保存先。`main=/mnt/main,sub=/mnt/sub`のように「ID=パス」をカンマで区切って指定する |
| `KOTATSU_SERVER_LISTEN` | 待ち受けアドレス。既定は`:8080` |
| `KOTATSU_SERVER_ALLOWED_NETWORKS` | 接続を受け付けるネットワーク。既定はプライベートアドレスだけ |

秘密情報は、ファイルのパスとしても渡せます。
その場合は、環境変数の名前の末尾に`_FILE`を付けてください（例: `KOTATSU_AUTH_SECRET_KEY_FILE`）。

### 起動と初回セットアップ

最初に、DBのスキーマを作ります。

```bash
kotatsu migrate
```

次に、全ロールを1つのプロセスで起動してください。

```bash
kotatsu all
```

管理者がまだいなければ、起動時のログにセットアップトークンが出力されます。
ブラウザで`http://<ホスト>:8080/setup`を開き、そのトークンを入力して最初の管理者を作ってください。
作った管理者でログインすると、認証アプリの登録を求められます。

## 開発

開発に使うツールは、[mise](https://mise.jdx.dev/)でインストールしてください。

```bash
mise install
```

| コマンド | 内容 |
| --- | --- |
| `mise run generate` | sqlc、gqlgen、GraphQLのスキーマなどの生成物を作り直し、マイグレーションを検証する |
| `mise run test` | Goのテストを`-race`付きで実行する |
| `mise run lint` | golangci-lintを実行する |
| `mise run web:check` | 画面のlintと型を確認する |
| `mise run web:test` | 画面のテストを実行する |
| `mise run db:diff -- <名前>` | `db/schema.sql`の変更からマイグレーションを作る |

DBを使うテストは、`KOTATSU_TEST_DATABASE_URL`にPostgreSQLの接続先を渡したときだけ実行されます。

## ライセンス

Kotatsuは[Apache License 2.0](LICENSE)で公開しています。
同梱するソフトウェアとデータのライセンスは、[設計書の付録B](docs/spec.v1.md#付録-b-ライセンスの境界)にまとめました。
