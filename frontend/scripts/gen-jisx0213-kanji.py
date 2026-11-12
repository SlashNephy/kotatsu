#!/usr/bin/env python3
# 字幕のフォントを分けるのに使う、JIS X 0213 で足された漢字の表 (scripts/jisx0213-kanji.json) を作る (#1520)。
# JIS X 0208 に無く (ISO-2022-JP で符号化できず)、JIS X 0213 にある (EUC-JIS-2004 で符号化できる) 漢字を集める。
# ARIB の追加漢字 (aribb24.js の追加記号の表) は、字幕でよく使うので、基本のファイルに残すよう除く。
# 表は符号化の規格だけで決まるので、作り直すのは、aribb24.js の表が変わったときだけでよい。Python の標準の符号化だけを使う。
#
# 使い方: python3 scripts/gen-jisx0213-kanji.py (frontend で、pnpm install の後に)
import json
import pathlib
import re
import unicodedata

root = pathlib.Path(__file__).resolve().parent.parent
arib = root / 'node_modules/aribb24.js/src/lib/tokenizer/b24/jis8/ARIB/symbol-unicode.ts'
arib_chars = set()
for literal in re.findall(r"'((?:[^'\\]|\\.)*)'", arib.read_text(encoding='utf-8')):
    arib_chars.update(ord(c) for c in literal)
if len(arib_chars) < 100:
    raise SystemExit(f'too few ARIB symbols in {arib}: {len(arib_chars)}')


def encodable(char: str, encoding: str) -> bool:
    try:
        char.encode(encoding)
    except UnicodeEncodeError:
        return False
    return True


# JIS X 0213 の第 2 面が使う区。Python の euc_jis_2004 は、3 バイト (0x8F で始まる) の符号で、第 2 面にない区
# (JIS X 0212 の区) の字も符号化するので、第 2 面の区のものだけを JIS X 0213 の字とみなす。
PLANE2_ROWS = {1, 3, 4, 5, 8, 12, 13, 14, 15, *range(78, 95)}


def in_jisx0213(char: str) -> bool:
    try:
        encoded = char.encode('euc_jis_2004')
    except UnicodeEncodeError:
        return False
    if len(encoded) == 3 and encoded[0] == 0x8F:
        return encoded[1] - 0xA0 in PLANE2_ROWS
    return len(encoded) == 2


kanji = []
for codepoint in range(0x3400, 0x30000):
    char = chr(codepoint)
    name = unicodedata.name(char, '')
    if not (name.startswith('CJK UNIFIED IDEOGRAPH') or name.startswith('CJK COMPATIBILITY IDEOGRAPH')):
        continue
    if codepoint in arib_chars or encodable(char, 'iso2022_jp') or not in_jisx0213(char):
        continue
    kanji.append(codepoint)

# 連続する字を、[始まり, 終わり] の範囲にまとめる。
ranges = []
for codepoint in kanji:
    if ranges and ranges[-1][1] == codepoint - 1:
        ranges[-1][1] = codepoint
    else:
        ranges.append([codepoint, codepoint])

output = root / 'scripts/jisx0213-kanji.json'
output.write_text(json.dumps({'count': len(kanji), 'ranges': ranges}, separators=(',', ':')) + '\n', encoding='utf-8')
print(f'wrote {len(kanji)} kanji in {len(ranges)} ranges to {output.relative_to(root)}')
