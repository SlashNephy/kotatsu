# 開発

このドキュメントは、手元でKotatsuを動かして開発する手順をまとめたものです。
運用のための手順は[セットアップ](setup.md)にあります。

## 必要なもの

- [mise](https://mise.jdx.dev/)：Go、Node.js、pnpmなどのツールを、決めた版で入れます。
- Docker（Compose v2を含む）：PostgreSQL、偽のチューナーサーバー、Kotatsuを動かします。

ツールは、リポジトリの直下で次のコマンドを実行して入れてください。
`mise install`は、そのディレクトリの`mise.toml`のツールだけを入れるので、`backend/`のツール（golangci-lint、sqlc、atlas）は`backend/`で入れます。

```bash
mise install
mise -C backend install
```

## Docker Composeで動かす

リポジトリの直下の[compose.yaml](../compose.yaml)は、開発のための構成です。
手元のソースをコンテナにマウントし、Kotatsuと画面の開発サーバーを動かします。
PostgreSQL、偽のチューナーサーバー、Dexも一緒に動かします。

```bash
mise run dev
```

`mise run dev`は、`docker compose up --build`に、手元のユーザーのUIDとGIDを渡して実行します。
コンテナの中で作ったファイルを、手元のユーザーの持ち物にするためです。

| サービス | 内容 | 手元からの接続先 |
| --- | --- | --- |
| `postgres` | PostgreSQL 18 | `127.0.0.1:55432`（ユーザーとパスワードはどちらも`postgres`） |
| `faketuner` | 偽のチューナーサーバー（`backend/cmd/faketuner`） | `127.0.0.1:40772` |
| `dex` | 開発用のOIDCのIdP（[Dex](https://dexidp.io/)） | `http://dex.localhost:5556/dex` |
| `kotatsu` | 全ロールを1つのコンテナで動かします | なし（`web`が中継します） |
| `web-install` | 画面の依存を入れます | なし |
| `web` | 画面の開発サーバー（Vite）。ブラウザはここを開きます | `http://localhost:8080` |
| `generate` | SQLとGraphQLのスキーマの変更から、生成物を作り直します | なし |
| `relay` | 画面のクエリの変更から、Relayの生成物を作り直します | なし |

偽のチューナーサーバーは、`backend/testdata/ts/test.ts`を、すべてのサービスのストリームとして流します。
番組表も、テスト用のものを返します。
実際のチューナーサーバーにつなぐ方法は、後述の「実際のチューナーサーバーにつなぐ」を見てください。

秘密キーと、Dexのユーザーのパスワードは、開発専用の固定の値です。
この構成を運用に使わないでください。運用の例は[deploy/compose/compose.yaml](../deploy/compose/compose.yaml)にあります。

DBと録画は、Composeのボリュームに残ります。
消して初めからやり直すときは、`docker compose down --volumes`を実行してください。

### ポートを変える

ほかのアプリが8080番を使っていれば、`KOTATSU_DEV_PORT`で変えられます。

```bash
KOTATSU_DEV_PORT=18080 mise run dev
```

### 実際のチューナーサーバーにつなぐ

`KOTATSU_DEV_TUNER_URL`にチューナーサーバーのURLを渡すと、偽のチューナーサーバーの代わりにつなぎます。
あわせて`KOTATSU_DEV_DB`に名前を渡し、DBと録画を別のボリュームに分けてください。

```bash
KOTATSU_DEV_TUNER_URL=http://192.0.2.10:40772/ KOTATSU_DEV_DB=real mise run dev
```

`KOTATSU_DEV_DB=real`なら、DBは`kotatsu-dev_postgres-real`、録画は`kotatsu-dev_recordings-real`のボリュームに置きます。
渡さなければ、`kotatsu-dev_postgres`と`kotatsu-dev_recordings`を使います。
偽のチューナーサーバーの番組表と、実際の番組表が混ざらないようにするためです。
偽のチューナーサーバーのサービスには、実際のサービスと同じIDのものもあります。

同じ名前を渡せば、前のデータの続きから使えます。
ボリュームを分けても、ポートは同じなので、2つを同時には動かせません。
切り替えるときは、`docker compose down`で止めてから起動し直してください。

実際のチューナーサーバーを、ほかのKotatsuやEPGStationと一緒に使っているときは、チューナーを取り合うことに気をつけてください。
開発環境でライブを見たり、ルールで録画したりすると、そのぶんのチューナーを使います。

### ログインする

`http://localhost:8080/login`を開き、「Dexでログイン」を押してください。
Dexの画面で、次のどれかのユーザーでログインします。
パスワードはどれも`password`です。

| メールアドレス | Kotatsuのロール |
| --- | --- |
| `admin@example.com` | 管理者 |
| `member@example.com` | メンバー |
| `viewer@example.com` | 視聴者 |

初めてログインすると、Kotatsuにユーザーが作られます。
ロールは、Dexのユーザーのグループで決まります。
OIDCでのログインには、認証アプリの登録が要りません。

ユーザーとパスワードは、[docker/dex/kotatsu.yaml](../docker/dex/kotatsu.yaml)で変えられます。
DexのissuerのURLは`dex.localhost`です。
ブラウザは`*.localhost`を手元のマシンに向けるので、そのまま開けます。

初回セットアップの画面（`/setup`）で、パスワードでログインする管理者を作ることもできます。
その場合は、`kotatsu`のログに出るセットアップ用のトークンを使ってください。

```bash
docker compose logs kotatsu | grep setup_token
```

パスワードでログインすると、認証アプリ（TOTP）の登録を求められます。
パスワードでのログインには、第二要素が必須だからです。

## ソースの変更を反映する

`mise run dev`で動かしている間は、ソースを保存すると、次のように反映されます。

| 変えたもの | 反映のしかた |
| --- | --- |
| Goのソース（`backend/`の`cmd/`、`internal/`） | `kotatsu`がビルドし直し、マイグレーションを当ててから起動し直します。数秒かかり、その間はAPIが502を返します |
| 画面のソース（`frontend/src/`） | 開発サーバーが、ページを読み込み直さずに差し替えます |
| SQLのクエリやスキーマ（`backend/db/queries/`、`backend/db/schema.sql`）、GraphQLのスキーマ（`backend/internal/web/graph/schema/`） | `generate`が、sqlcとgqlgenの生成物と`frontend/schema.graphql`を作り直します。続けて、Goのビルドと`relay`が動きます |
| 画面のGraphQLのクエリ | `relay`が、`__generated__`を作り直します |

マイグレーションは自動では作りません。
`backend/db/schema.sql`を変えたら、`mise run //backend:db:diff -- <名前>`でマイグレーションを作ってください。

ビルドに失敗したときは、`docker compose logs -f kotatsu`でエラーを確かめてください。
直して保存すると、またビルドします。

`frontend/pnpm-lock.yaml`を変えたときは、依存を入れ直してから、開発サーバーを起動し直してください。

```bash
docker compose up web-install
docker compose restart web
```

`Dockerfile`や、ルートと`backend/`と`frontend/`の`mise.toml`、`mise.dev.toml`を変えたときは、`mise run dev`を実行し直してください。
開発用のイメージを作り直します。

`mise run //frontend:dev`は、Composeの外で画面の開発サーバーを`http://localhost:5173`に起動します。
Composeを使わずに、手元でKotatsuを`127.0.0.1:8080`に起動しているときに使います。

## テストを流す

DBを使うGoのテストは、`KOTATSU_TEST_DATABASE_URL`にPostgreSQLの接続先を渡したときだけ実行されます。
Composeの`postgres`サービスに向けられます。

```bash
KOTATSU_TEST_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:55432/postgres?sslmode=disable' mise run //backend:test
```

テストは、テストごとに一時的なDBを作って使います。
Composeで動かしているKotatsuのDB（`kotatsu`）には触れません。

ほかのコマンドは、[README](../README.md#開発)の表にまとめました。
