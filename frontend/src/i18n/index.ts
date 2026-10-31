import i18next from 'i18next';
import { initReactI18next } from 'react-i18next';

import en from './en.json';
import ja from './ja.json';
import { detectLanguage } from './language';

// 文言は i18next を通す (設計書 10.1)。言語は、ブラウザの言語の設定から選ぶ (#831)。
export const resources = { ja: { translation: ja }, en: { translation: en } } as const;

// navigator がない環境 (テストなど) では、日本語にする。
const preferred = typeof navigator === 'undefined' ? ['ja'] : navigator.languages;

void i18next.use(initReactI18next).init({
  lng: detectLanguage(preferred),
  // 英語の訳がない文言は、日本語で出す。
  fallbackLng: 'ja',
  resources,
  interpolation: { escapeValue: false },
});

// 読み上げや、ブラウザの翻訳の判定が、画面の言語に合うようにする。
if (typeof document !== 'undefined') {
  document.documentElement.lang = i18next.language;
}

export default i18next;
