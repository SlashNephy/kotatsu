// デバイス認可 (RFC 8628、#1527、設計書 9.9) の user_code。tenban の oauth.NormalizeUserCode と同じく正規化する。

// userCodePattern は、ハイフンと空白を除いた user_code。英大文字と数字から、見間違えやすい 0、O、1、I を除いた 32 字を、8 個並べる。
const userCodePattern = /^[2-9A-HJ-NP-Z]{8}$/;

// normalizeUserCode は、人が打った user_code を、ハイフンで区切った形に正規化する。小文字、全角の字、
// ハイフンの抜けや余り、空白を許す。字数が違うか、使わない字を含めば null を返す。
export function normalizeUserCode(input: string): string | null {
  const compact = input.normalize('NFKC').toUpperCase().replace(/[-\s]/g, '');
  if (!userCodePattern.test(compact)) {
    return null;
  }

  return `${compact.slice(0, 4)}-${compact.slice(4)}`;
}
