// 画面の言語 (#831)。ブラウザの言語の設定 (navigator.languages) の順に、対応する言語を選ぶ。
// 画面の文言だけを訳し、サーバーが作る文 (通知、Web Push、Webhook、エラー) は日本語のままにする。

export const languages = ['ja', 'en'] as const;
export type Language = (typeof languages)[number];

// detectLanguage は、ブラウザの言語の設定から、画面の言語を選ぶ。対応する言語がなければ英語にする。
// 「ja-JP」や「en_GB」のような地域つきの名前は、言語の部分で比べる。
export function detectLanguage(preferred: readonly (string | undefined)[]): Language {
  for (const tag of preferred) {
    const language = tag?.toLowerCase().split(/[-_]/)[0];
    const found = languages.find((candidate) => candidate === language);
    if (found !== undefined) {
      return found;
    }
  }

  return 'en';
}

// dateLocale は、日時を書く書式の言語 (Intl のロケール) を返す。
export function dateLocale(language: string): string {
  return language === 'en' ? 'en-US' : 'ja-JP';
}
