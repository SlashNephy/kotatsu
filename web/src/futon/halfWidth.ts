// EPG の文字列 (番組名、サービス名、番組の説明) は、英数字を全角で送ってくる (ARIB の慣習)。
// 本文のフォントは全角の英数字を全角の幅で描き、間延びして見えるので、画面に出すときに半角へ直す。

// convertible は、半角へ直す全角の文字。英数字と、英数字に添えて使う記号だけにする。
// 「！」「？」「（）」「～」や全角の空白は、和文の約物や区切りとして使うので、全角のまま残す。
const convertible = /[Ａ-Ｚａ-ｚ０-９＃＄％＆＊＋－．／：；＝＠＿]/g;

// toHalfWidth は、text の全角の英数字と記号を、半角にする。
export function toHalfWidth(text: string): string {
  return text.replace(convertible, (char) => String.fromCharCode(char.charCodeAt(0) - 0xfee0));
}
