// ビルドのときに vite.config.ts のプラグインが作る、並べ終えた辞書のインデックスの JSON の文字列。
declare module 'virtual:zxcvbn-precomputed-dictionary' {
  const json: string;
  export default json;
}
