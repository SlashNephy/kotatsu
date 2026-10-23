# Kotatsuへの貢献

Kotatsuに関心を持っていただき、ありがとうございます。
不具合の報告、機能の提案、プルリクエストを歓迎します。

## 不具合の報告と機能の提案

- [Issue](https://github.com/SlashNephy/kotatsu/issues/new/choose)から、テンプレートに沿って書いてください。
- 使い方の質問は、[Discussions](https://github.com/SlashNephy/kotatsu/discussions)へお願いします。
- 脆弱性は、Issueではなく、[セキュリティポリシー](SECURITY.md)に沿って非公開で知らせてください。

## 開発の準備

手元で動かす手順は、[開発](../docs/development.md)にまとめています。
ツールは[mise](https://mise.jdx.dev/)で入れます。

```bash
mise install
mise -C backend install
mise run dev
```

## 変更を送る前に

次の確認がすべて通ることを確かめてから、プルリクエストを送ってください。

| コマンド | 内容 |
| --- | --- |
| `mise run //backend:generate` | 生成物 (sqlc、gqlgen、GraphQLのスキーマなど) を作り直す。生成物の差分も一緒にコミットする |
| `mise run //backend:lint` | golangci-lintを実行する |
| `mise run //backend:test` | Goのテストを実行する。DBを使うテストには`KOTATSU_TEST_DATABASE_URL`が要る |
| `mise run //frontend:check` | 画面のlintと型を確かめる |
| `mise run //frontend:test` | 画面のテストを実行する |

## 設計書

振る舞い、仕様、既定値を変える変更では、同じプルリクエストで[設計書](../docs/spec.v1.md)の該当する節も直してください。
表 (既定値の一覧など) や、同じことを書いた別の節も忘れずに直します。

## コミットとプルリクエスト

- コミットメッセージは[Conventional Commits](https://www.conventionalcommits.org/ja/v1.0.0/)の形式で書きます (例: `feat: 番組表で日付を選べるようにする`)。
- プルリクエストのタイトルも Conventional Commits の形式で書きます。タイトルの種類 (`feat`、`fix`、`perf`、`docs`) から、リリースノートで分けるためのラベルが付きます。
- コード内のコメント、コミット、プルリクエストは日本語で、ログとエラーメッセージは英語で書きます。
- プルリクエストは、できるだけ小さく保ってください。
- 画面を変えたときは、変更の前と後が分かるスクリーンショットを添えてください。放送の映像が写るときは、ぼかしてください。

## 行動規範

このプロジェクトに参加する人は、[行動規範](CODE_OF_CONDUCT.md)を守ってください。
