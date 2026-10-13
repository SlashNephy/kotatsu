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
| `faketuner` | 偽のチューナーサーバー（`cmd/faketuner`） | `127.0.0.1:40772` |
| `dex` | 開発用のOIDCのIdP（[Dex](https://dexidp.io/)） | `http://dex.localhost:5556/dex` |
| `kotatsu` | 全ロールを1つのコンテナで動かします | なし（`web`が中継します） |
| `web-install` | 画面の依存を入れます | なし |
| `web` | 画面の開発サーバー（Vite）。ブラウザはここを開きます | `http://localhost:8080` |

偽のチューナーサーバーは、`testdata/ts/test.ts`を、すべてのサービスのストリームとして流します。
番組表も、テスト用のものを返します。
実際のチューナーサーバーにつなぐときは、`kotatsu`サービスの`KOTATSU_TUNER_URL`を書き換えてください。

秘密キーと、Dexのユーザーのパスワードは、開発専用の固定の値です。
この構成を運用に使わないでください。運用の例は[deploy/compose/compose.yaml](../deploy/compose/compose.yaml)にあります。

DBと録画は、Composeのボリュームに残ります。
消して初めからやり直すときは、`docker compose down --volumes`を実行してください。

### ポートを変える

ほかのアプリが8080番を使っていれば、`KOTATSU_DEV_PORT`で変えられます。

```bash
KOTATSU_DEV_PORT=18080 mise run dev
```

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

## ソースの変更を反映する

`mise run dev`で動かしている間は、ソースを保存すると、次のように反映されます。

| 変えたもの | 反映のしかた |
| --- | --- |
| Goのソース（`cmd/`、`internal/`、`graph/`） | `kotatsu`がビルドし直し、マイグレーションを当ててから起動し直します。数秒かかり、その間はAPIが502を返します |
| 画面のソース（`web/src/`） | 開発サーバーが、ページを読み込み直さずに差し替えます |

ビルドに失敗したときは、`docker compose logs -f kotatsu`でエラーを確かめてください。
直して保存すると、またビルドします。

`web/pnpm-lock.yaml`を変えたときは、依存を入れ直してから、開発サーバーを起動し直してください。

```bash
docker compose up web-install
docker compose restart web
```

`Dockerfile`や`mise.toml`、`mise.dev.toml`を変えたときは、`mise run dev`を実行し直してください。
開発用のイメージを作り直します。

`mise run web:dev`は、Composeの外で画面の開発サーバーを`http://localhost:5173`に起動します。
Composeを使わずに、手元でKotatsuを`127.0.0.1:8080`に起動しているときに使います。

## テストを流す

DBを使うGoのテストは、`KOTATSU_TEST_DATABASE_URL`にPostgreSQLの接続先を渡したときだけ実行されます。
Composeの`postgres`サービスに向けられます。

```bash
KOTATSU_TEST_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:55432/postgres?sslmode=disable' mise run test
```

テストは、テストごとに一時的なDBを作って使います。
Composeで動かしているKotatsuのDB（`kotatsu`）には触れません。

ほかのコマンドは、[README](../README.md#開発)の表にまとめました。
