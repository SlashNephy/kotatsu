#!/usr/bin/env python3
"""PR で足した行に、使わない語の一覧 (.yomiyasu-words/) の語がないかを確認する (#2131、docs/writing.md)。

使い方: scripts/check-words.py <yomiyasu のスキルのフォルダ> <比較の元のコミット>

比較の元のコミットから HEAD までの差分で、足した行と書き換えた行だけを見る。コミットしていない変更は見ない。
一覧の語に当たる行があれば、GitHub Actions の注釈の形で出して、終了コード 1 で終わる。

yomiyasu の lint は Markdown として読むので、字下げした行 (コードブロック)、< で始まる行 (HTML のブロック)、
``` (コードの囲み) を含む行の後ろを読み飛ばす。そのため、各行の行頭の空白を除き、行頭に "x " を足し、
バッククォートを ' にしてから渡す。行の数は変えないので、指摘の行番号は元のファイルの行番号と同じになる。
"""

import json
import os
import re
import subprocess
import sys

# 見ないファイル。生成物、第三者の文書、英語の画面の文言、一覧そのもの
SKIP = re.compile(
    r"(^|/)(generated|testdata|node_modules|__generated__)/"
    r"|\.gen\.|_gen\.go$|_enum\.go$|/sqlc/"
    r"|^frontend/schema\.graphql$"
    r"|^frontend/src/i18n/en\.json$"
    r"|^\.yomiyasu-words/"
    r"|(^|/)LICENSE[^/]*$|\.LICENSE$|COPYING"
    r"|^frontend/public/licenses/"
)
JAPANESE = re.compile(r"[぀-ヿ一-鿿]")
HUNK = re.compile(r"^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@")


def added_lines(base: str) -> dict[str, set[int]]:
    """base から HEAD までで、足した行と書き換えた行の番号をファイルごとに返す。"""
    # 手元の git の設定 (パスの引用、接頭辞、外部の diff) で、差分の形が変わらないようにする
    out = subprocess.run(
        ["git", "-c", "core.quotePath=false", "diff", "--no-ext-diff", "--src-prefix=a/", "--dst-prefix=b/",
         "-U0", "--no-color", "--no-renames", "--diff-filter=AM", base, "HEAD"],
        check=True, capture_output=True, text=True, encoding="utf-8", errors="replace",
    ).stdout
    result: dict[str, set[int]] = {}
    path = None
    in_header = False
    for line in out.split("\n"):
        # ファイルの見出しは diff --git から最初の @@ までの間だけにある。内容の行が「++ 」で始まっても、見出しと取り違えない
        if line.startswith("diff --git "):
            in_header, path = True, None
        elif in_header and line.startswith("+++ "):
            path = line[6:] if line.startswith("+++ b/") else None
        elif m := HUNK.match(line):
            in_header = False
            start, count = int(m.group(1)), int(m.group(2) or "1")
            if path is not None and count:
                result.setdefault(path, set()).update(range(start, start + count))
    return result


def read_head(path: str) -> str | None:
    """HEAD のファイルの内容。差分と同じコミットから読み、作業ツリーの変更で行番号がずれないようにする。"""
    out = subprocess.run(["git", "show", f"HEAD:{path}"], capture_output=True)
    if out.returncode != 0:
        return None
    try:
        return out.stdout.decode("utf-8")
    except UnicodeDecodeError:
        return None


def lint(lint_path: str, words: str, text: str) -> list[dict]:
    """行ごとに整えた text を yomiyasu の lint に渡し、一覧の語の指摘 (project_term) を返す。"""
    prepared = "\n".join("x " + line.lstrip(" \t").replace("`", "'") for line in text.split("\n"))
    out = subprocess.run(
        [sys.executable, lint_path, "--words", words, "--json", "-"],
        input=prepared, capture_output=True, text=True, encoding="utf-8",
    )
    if out.returncode not in (0, 1):
        raise RuntimeError(f"yomiyasu_lint.py failed: {out.stderr.strip()}")
    return [f for f in json.loads(out.stdout)["findings"] if f["rule"] == "project_term"]


def main() -> int:
    if len(sys.argv) != 3:
        print(__doc__.strip().split("\n")[2], file=sys.stderr)
        return 2
    skill, base = sys.argv[1], sys.argv[2]
    lint_path = os.path.join(skill, "scripts", "yomiyasu_lint.py")
    words = os.path.abspath(".yomiyasu-words")
    found = 0
    for path, lines in sorted(added_lines(base).items()):
        if SKIP.search(path):
            continue
        text = read_head(path)
        if text is None or not JAPANESE.search(text):
            continue
        source = text.split("\n")
        for finding in lint(lint_path, words, text):
            line = finding["line"]
            if line not in lines:
                continue
            found += 1
            message = finding["message"].replace("%", "%25").replace("\r", "%0D").replace("\n", "%0A")
            print(f"::error file={path},line={line}::{message}")
            print(f"{path}:{line}: {source[line - 1].strip()}")
    if found:
        print(f"\n{found} line(s) use words listed in .yomiyasu-words/. "
              "Rewrite them as described in docs/writing.md.")
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
