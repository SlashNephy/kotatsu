// パスワードマネージャーに、この入力欄を埋めさせない属性 (#1709)。
// 番組名やルールの条件の欄に、ユーザー名やパスワードが入るのを防ぐ。
// 1Password と Bitwarden は属性があるかどうかを、LastPass は値が "true" かを見る。
export const passwordManagerIgnoreProps = {
  'data-1p-ignore': 'true',
  'data-lpignore': 'true',
  'data-bwignore': 'true',
} as const;
