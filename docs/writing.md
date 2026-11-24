# 文章の書き方

このドキュメントはKotatsuの日本語の文章を書くときの決まりをまとめたものです。
人が書くときも、エージェントが書くときも同じ決まりに従います。

## 対象

次の文章に当てます。

- 画面の文言（`frontend/src/i18n/ja.json`）
- ドキュメント（`README.md`、`docs/`、`.github/`）
- GraphQLのスキーマの説明、MCPのツールの説明、通知の文
- コードのコメント
- コミットメッセージ、IssueとPRのタイトルと本文

コミットメッセージ、PRとIssueのタイトルの形式は、[CONTRIBUTING.md](../.github/CONTRIBUTING.md)と[AGENTS.md](../AGENTS.md)の「コミットと PR」にあります。

## 使わない語

使わない語は[.yomiyasu-words/](../.yomiyasu-words/)の2つのファイルにまとめています。

- [flag.txt](../.yomiyasu-words/flag.txt)：使わない語と、代わりに使う語の組
- [flag-patterns.txt](../.yomiyasu-words/flag-patterns.txt)：漢字1字の語を、熟語を除いて当てる正規表現

どちらのファイルでも、語の上の注釈に場面ごとの使い分けを書いています。
使い分けは語のすぐそばに書き、このドキュメントには同じことを書きません。

### lintの流し方

リポジトリの中で[yomiyasu](https://github.com/nanaism/yomiyasu)のlintを流すと、lintがこの一覧を自動で読みます。
一覧の語に当たった行には`project_term`の指摘が出て、`flag.txt`に使う語があれば、指摘に添えます。

```bash
python3 <yomiyasuのフォルダ>/scripts/yomiyasu_lint.py docs/setup.md
```

指摘は`info`なので、lintの終了コードは変わりません。

lintは行頭を4つ以上の空白かタブで字下げした行をMarkdownのコードブロックとして読まないので、その行には指摘が出ません。
コードのコメントを検査するときは、行頭の空白を除いてから標準入力で渡します。行の数は変わらないので、指摘の行番号は元のファイルの行番号と同じです。

```bash
sed -E 's/^[[:space:]]+//' backend/internal/usecase/recording/recorder.go | python3 <yomiyasuのフォルダ>/scripts/yomiyasu_lint.py -
```

### CIの検査

PRではCIの`words`のジョブが、PRで足した行と書き換えた行に一覧の語がないかを確認し、あれば落とします（#2131）。
検査するのは`scripts/check-words.py`で、yomiyasuは`scripts/yomiyasu.SOURCE`に固定したコミットを使います。
生成物、英語の画面の文言、ライセンスの文書は見ません。

手元でも、比較の元のコミットを渡して同じ検査を流せます。

```bash
python3 scripts/check-words.py <yomiyasuのスキルのフォルダ> origin/main
```

規格の番号（「表 2-22」）のように、一覧の語に当たっても書き換えられない語を含む行を変えたときは、その語を言い換えるか、PRで尋ねます。

### 指摘が出たときの書き直し方

1. 指摘の出た語が使う語と同じものを指すなら、使う語に替えます。
2. 場面で替える語が変わるなら、一覧のその語の注釈を読み、文に合う語を選びます（「入口」は、HTTPのURLなら「エンドポイント」、mainパッケージなら「エントリーポイント」）。
3. 注釈のどれにも当たらなければ、前後の文から、何が何をするかを書いた文に直します。
4. それでも決まらなければ、本文を残し、PRで書き手に尋ねます。

語を替えるだけで文が読めなくなるときは、文ごと書き直します。
一覧の語は定着した言い方に見えても残しません。

### 一覧に語を足すとき

1. 足す語を一覧に入れ、対象の文章にlintを流して、当たった行を読みます。
2. 普通の言い方への当たりが多ければ、語を長くするか、`flag-patterns.txt`で前後の字を除きます。
3. 場面で替える語が変わるなら、語の上に注釈を書きます。
4. 漢字1字の語はほかの語の中でも当たるので、`flag-patterns.txt`に書きます。

`flag.txt`の区切りはタブです。空白で区切ると、行全体が1つの語になり、当たらなくなります。
