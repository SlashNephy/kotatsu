# AGENTS.md

## 設計書

設計は [docs/spec.v1.md](docs/spec.v1.md) にまとめています。

- 振る舞い、仕様、既定値を変える変更では、同じ PR で設計書の該当する節も直します。表（既定値の一覧など）や、同じことを書いた別の節も忘れずに直します。
- 設計書と実装が食い違っているのに気づいたら、どちらが正しいかを確かめ、直すか Issue にします。
- 着手する前に、関わる節を読みます。

## 範囲

- スクランブルを解除する機能と、カードリーダーを扱う機能は足しません（設計書 3.1、[CONTRIBUTING.md](.github/CONTRIBUTING.md)）。

## 入口（main パッケージ）

- `backend/cmd/` には、配るもの（`kotatsu`）だけを置きます。開発や検証にだけ使う入口（偽のサーバーなど）は、`backend/internal/cmd/<名前>` に置きます（設計書 11.5）。

## 破壊的変更

- 更新した利用者に対処が要る変更（既定値を変えて今の動きが変わる、データを消し始める、リバースプロキシなどの構成を変える必要があるなど）では、同じ PR で [docs/breaking-changes.md](docs/breaking-changes.md) に書きます。
- 書く先は、先頭の版の節です。先頭の節が既にリリースした版なら、その次の版の節を足します。月をまたいでリリースすると版がずれるので、リリースの前に見出しを直します（設計書 11.1）。

## プライバシー

- 外部との通信を増やす変更や、保存する個人情報を増やす変更では、同じ PR で [docs/privacy.md](docs/privacy.md) の一覧も直します。

## タスク

ツールとタスクは、mise の monorepo の構成で、ルートと `backend/`、`frontend/` の `mise.toml` に分けています。

- `backend/` と `frontend/` のタスクは、`mise run //backend:test` や `mise run //frontend:check` のように呼びます。ルートのタスクは `mise run build` のように呼びます。
- `mise install` は、そのディレクトリの `mise.toml` のツールだけを入れます。`backend/` のツールは `mise -C backend install` で入れます。

## フロントエンド

- React Compiler を有効にしているので、`useMemo` と `useCallback` は新しく書きません（#1684）。値や関数をそのまま書けば、コンパイラが覚えます。
- コンパイラが最適化を諦めたコンポーネント（描画の中で ref を読むなど）では、手で書いた `useMemo` と `useCallback` を外しません。外すと、描くたびに作り直し、それを依存に持つ effect が繰り返し走ります。外す前に、そのコンポーネントがコンパイラを通るかを確かめます。

## 日本語の語彙

- 画面の文言、ドキュメント、コードのコメント、コミット、Issue と PR の日本語には、[.yomiyasu-words/](.yomiyasu-words/) の一覧にある語を使いません。一覧に使う語があれば、それを使います。
- 一覧は yomiyasu の lint が読みます。リポジトリの中で lint を流すと、一覧の語に当たった行に指摘が出ます。

## コミットと PR

- コミットメッセージと PR のタイトルは、Conventional Commits の形式で書きます（[CONTRIBUTING.md](.github/CONTRIBUTING.md)）。PR のタイトルの種類（`feat`、`fix`、`perf`、`docs`）から、リリースノートで分けるためのラベルが付くので、変更の中身に合った種類を選びます。
- Issue のタイトルは、Conventional Commits の形式にせず、日本語の文で書きます。種類のラベル（`bug`、`enhancement`）を付けます。
- 1 つの PR で足すマイグレーションのファイルは、1 つまでにします。PR の途中でスキーマを直すときも、新しいファイルを足さず、その PR のファイルを直します。main と衝突したら、main の最新より後の時刻に改名して `atlas migrate hash` を流し直します。
