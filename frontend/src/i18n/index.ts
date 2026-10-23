import i18next from 'i18next';
import { initReactI18next } from 'react-i18next';

import ja from './ja.json';

// v1 は日本語だけだが、後から仕組みを入れ直さずに済むよう、最初から i18next を通す (設計書 10.1)。
export const resources = { ja: { translation: ja } } as const;

void i18next.use(initReactI18next).init({
  lng: 'ja',
  fallbackLng: 'ja',
  resources,
  interpolation: { escapeValue: false },
});

export default i18next;
