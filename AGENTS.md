# AGENTS.md

## 設計書

設計は [docs/spec.v1.md](docs/spec.v1.md) にまとめています。

- 振る舞い、仕様、既定値を変える変更では、同じ PR で設計書の該当する節も直します。表（既定値の一覧など）や、同じことを書いた別の節も忘れずに直します。
- 設計書と実装が食い違っているのに気づいたら、どちらが正しいかを確かめ、直すか Issue にします。
- 着手する前に、関わる節を読みます。

## 破壊的変更

- 更新した利用者に対処が要る変更（既定値を変えて今の動きが変わる、データを消し始める、リバースプロキシなどの構成を変える必要があるなど）では、同じ PR で [docs/breaking-changes.md](docs/breaking-changes.md) に書きます。
- 書く先は、先頭の版の節です。先頭の節が既にリリースした版なら、その次の版の節を足します。月をまたいでリリースすると版がずれるので、リリースの前に見出しを直します（設計書 11.1）。

## プライバシー

- 外部との通信を増やす変更や、保存する個人情報を増やす変更では、同じ PR で [docs/privacy.md](docs/privacy.md) の一覧も直します。

## タスク

ツールとタスクは、mise の monorepo の構成で、ルートと `backend/`、`frontend/` の `mise.toml` に分けています。

- `backend/` と `frontend/` のタスクは、`mise run //backend:test` や `mise run //frontend:check` のように呼びます。ルートのタスクは `mise run build` のように呼びます。
- `mise install` は、そのディレクトリの `mise.toml` のツールだけを入れます。`backend/` のツールは `mise -C backend install` で入れます。

## コミットと PR

- コミットメッセージと PR のタイトルは、Conventional Commits の形式で書きます（[CONTRIBUTING.md](.github/CONTRIBUTING.md)）。PR のタイトルの種類（`feat`、`fix`、`perf`、`docs`）から、リリースノートで分けるためのラベルが付くので、変更の中身に合った種類を選びます。
- Issue のタイトルは、Conventional Commits の形式にせず、日本語の文で書きます。種類のラベル（`bug`、`enhancement`）を付けます。
