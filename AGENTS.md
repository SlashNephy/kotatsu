# AGENTS.md

## 設計書

設計は [docs/spec.v1.md](docs/spec.v1.md) にまとめています。

- 振る舞い、仕様、既定値を変える変更では、同じ PR で設計書の該当する節も直します。表（既定値の一覧など）や、同じことを書いた別の節も忘れずに直します。
- 設計書と実装が食い違っているのに気づいたら、どちらが正しいかを確かめ、直すか Issue にします。
- 着手する前に、関わる節を読みます。

## タスク

ツールとタスクは、mise の monorepo の構成で、ルートと `backend/`、`frontend/` の `mise.toml` に分けています。

- `backend/` と `frontend/` のタスクは、`mise run //backend:test` や `mise run //frontend:check` のように呼びます。ルートのタスクは `mise run build` のように呼びます。
- `mise install` は、そのディレクトリの `mise.toml` のツールだけを入れます。`backend/` のツールは `mise -C backend install` で入れます。

## コミットと PR

- コミットメッセージと PR のタイトルは、Conventional Commits の形式で書きます（[CONTRIBUTING.md](.github/CONTRIBUTING.md)）。PR のタイトルの種類（`feat`、`fix`、`perf`、`docs`）から、リリースノートで分けるためのラベルが付くので、変更の中身に合った種類を選びます。
- Issue のタイトルは、Conventional Commits の形式にせず、日本語の文で書きます。種類のラベル（`bug`、`enhancement`）を付けます。
