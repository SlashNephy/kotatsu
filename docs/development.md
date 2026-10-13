# 開発

このドキュメントは、手元でKotatsuを動かして開発する手順をまとめたものです。
運用のための手順は[セットアップ](setup.md)にあります。

## 必要なもの

- [mise](https://mise.jdx.dev/)：Go、Node.js、pnpmなどのツールを、決めた版で入れます。
- Docker（Compose v2を含む）：PostgreSQL、偽のチューナーサーバー、Kotatsuを動かします。

ツールは、リポジトリの直下で次のコマンドを実行して入れてください。

```bash
mise install
```

## Docker Composeで動かす

リポジトリの直下の[compose.yaml](../compose.yaml)は、開発のための構成です。
手元のソースからKotatsuをビルドし、PostgreSQLと偽のチューナーサーバーと一緒に動かします。

```bash
docker compose up --build
```

| サービス | 内容 | 手元からの接続先 |
| --- | --- | --- |
| `postgres` | PostgreSQL 18 | `127.0.0.1:55432`（ユーザーとパスワードはどちらも`postgres`） |
| `faketuner` | 偽のチューナーサーバー（`cmd/faketuner`） | `127.0.0.1:40772` |
| `dex` | 開発用のOIDCのIdP（[Dex](https://dexidp.io/)） | `http://dex.localhost:5556/dex` |
| `migrate` | 起動の前に、DBのスキーマを最新にします | なし |
| `kotatsu` | 全ロールを1つのコンテナで動かします | `127.0.0.1:8080` |

偽のチューナーサーバーは、`testdata/ts/test.ts`を、すべてのサービスのストリームとして流します。
番組表も、テスト用のものを返します。
実際のチューナーサーバーにつなぐときは、`kotatsu`サービスの`KOTATSU_TUNER_URL`を書き換えてください。

秘密キーと、Dexのユーザーのパスワードは、開発専用の固定の値です。
この構成を運用に使わないでください。運用の例は[deploy/compose/compose.yaml](../deploy/compose/compose.yaml)にあります。

ソースを変えたら、`docker compose up --build`でビルドし直してください。
DBと録画は、Composeのボリュームに残ります。
消して初めからやり直すときは、`docker compose down --volumes`を実行してください。

### ポートを変える

ほかのアプリが8080番を使っていれば、`KOTATSU_DEV_PORT`で変えられます。

```bash
KOTATSU_DEV_PORT=18080 docker compose up --build
```

画面の開発サーバー（後述）にも、同じ値を渡してください。

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

ユーザーとパスワードは、[dev/dex.yaml](../dev/dex.yaml)で変えられます。
DexのissuerのURLは`dex.localhost`です。
ブラウザは`*.localhost`を手元のマシンに向けるので、そのまま開けます。

初回セットアップの画面（`/setup`）で、パスワードでログインする管理者を作ることもできます。
その場合は、`kotatsu`のログに出るセットアップ用のトークンを使ってください。

```bash
docker compose logs kotatsu | grep setup_token
```

パスワードでログインすると、認証アプリ（TOTP）の登録を求められます。
パスワードでのログインには、第二要素が必須だからです。

## 画面を開発する

画面だけを変えるときは、Viteの開発サーバーを使うと、保存した変更がすぐに反映されます。
Kotatsuを`docker compose up`で動かしたまま、次のコマンドを実行してください。

```bash
mise run web:dev
```

開発サーバーは、`/graphql`と`/auth`を`127.0.0.1:8080`のKotatsuに中継します。
ポートを変えたときは、`KOTATSU_DEV_PORT=18080 mise run web:dev`のように、同じ値を渡してください。

## テストを流す

DBを使うGoのテストは、`KOTATSU_TEST_DATABASE_URL`にPostgreSQLの接続先を渡したときだけ実行されます。
Composeの`postgres`サービスに向けられます。

```bash
KOTATSU_TEST_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:55432/postgres?sslmode=disable' mise run test
```

テストは、テストごとに一時的なDBを作って使います。
Composeで動かしているKotatsuのDB（`kotatsu`）には触れません。

ほかのコマンドは、[README](../README.md#開発)の表にまとめました。
