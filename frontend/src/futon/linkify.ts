// 番組の説明には、番組のホームページなどの URL が書かれることがある。押して開けるよう、文字とリンクに分ける。
// 説明は toHalfWidth で半角に直してから渡す。全角の URL (ｈｔｔｐｓ：／／…) も、半角に直せば拾える。

export type TextSegment =
  | { kind: 'text'; text: string }
  | { kind: 'link'; text: string; href: string };

// urlPattern は、説明の中の URL。http(s):// で始まるものと、スキームのない www. で始まるもの。
// 英数字の続きの途中から始まるもの (abcwww.…) は、URL とみなさない。
// 全角の「？」は、toHalfWidth が和文の約物として残すので、URL の中では ? とみなして続ける。
const urlPattern =
  /(?<![A-Za-z0-9])(?:https?:\/\/|www\.)[A-Za-z0-9][A-Za-z0-9\-._~:/?#[\]@!$&'()*+,;=%？]*/g;
// trailingPunctuation は、URL の末尾に付いた句読点。文の区切りとみなして、URL から外す。
const trailingPunctuation = /[.,;:!?'？]+$/;

// splitLinks は、text を、文字とリンクの並びに分ける。
export function splitLinks(text: string): TextSegment[] {
  const segments: TextSegment[] = [];
  let last = 0;
  for (const match of text.matchAll(urlPattern)) {
    const url = trimURL(match[0]);
    const start = match.index;
    if (start > last) {
      segments.push({ kind: 'text', text: text.slice(last, start) });
    }
    segments.push({ kind: 'link', text: url, href: hrefOf(url) });
    last = start + url.length;
  }
  if (last < text.length) {
    segments.push({ kind: 'text', text: text.slice(last) });
  }

  return segments;
}

// trimURL は、URL の末尾から、句読点と、対になる開き括弧のない閉じ括弧を外す。
// (… https://example.com/) のように、URL を括弧で囲んだ文に備える。
function trimURL(url: string): string {
  let trimmed = url;
  for (;;) {
    const next = trimmed.replace(trailingPunctuation, '');
    if (next.endsWith(')') && count(next, '(') < count(next, ')')) {
      trimmed = next.slice(0, -1);
    } else if (next.endsWith(']') && count(next, '[') < count(next, ']')) {
      trimmed = next.slice(0, -1);
    } else {
      return next;
    }
  }
}

function count(text: string, char: string): number {
  return text.split(char).length - 1;
}

// hrefOf は、リンクの行き先。スキームのない www. には https:// を補い、全角の「？」は ? に直す。
function hrefOf(url: string): string {
  const href = url.replaceAll('？', '?');

  return href.startsWith('www.') ? `https://${href}` : href;
}
