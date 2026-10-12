// ビルドのときに vite.config.ts のプラグインが作る、順位を付け終えた辞書の JSON の文字列。
declare module 'virtual:zxcvbn-ranked-dictionary' {
  const json: string;
  export default json;
}
