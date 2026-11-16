// 画面の中で語を探すときは、全角と半角、大文字と小文字の違いを無視する。
// 全角の英数字で入っている EPG の文字列やルールの名前を、半角で打った語でも見つけられるようにするためである。

// normalizeForSearch は、語と照らすために、text を NFKC で正規化し、小文字にする。
export function normalizeForSearch(text: string): string {
  return text.normalize('NFKC').toLowerCase();
}

// searchWords は、検索の入力を、空白 (全角の空白を含む) で区切った、正規化した語にする。空の語は捨てる。
export function searchWords(query: string): string[] {
  return normalizeForSearch(query)
    .split(/\s+/)
    .filter((word) => word !== '');
}

// containsAllWords は、正規化した haystack が、words のすべてを含むかどうか。words が空なら true。
export function containsAllWords(haystack: string, words: readonly string[]): boolean {
  return words.every((word) => haystack.includes(word));
}
