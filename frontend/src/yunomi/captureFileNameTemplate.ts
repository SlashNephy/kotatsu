import { useSyncExternalStore } from 'react';

import { browserStorage } from './volume';

// キャプチャのファイル名のテンプレート (#500、設計書 7.5)。保存の仕方は端末ごとに違うので、端末ごとに覚える。
// 空なら、既定の形 (日時 - サービス名 - 番組名) にする。

const storageKey = 'kotatsu.capture.fileNameTemplate';
// changeEvent は、同じタブの中で変えたことを知らせるイベント。storage イベントは、ほかのタブにしか届かない。
const changeEvent = 'kotatsu:capture-file-name-template';

// loadCaptureFileNameTemplate は、覚えたテンプレートを読む。覚えていないか、読めなければ空を返す。
export function loadCaptureFileNameTemplate(storage: Pick<Storage, 'getItem'> | null): string {
  try {
    return storage?.getItem(storageKey) ?? '';
  } catch {
    return '';
  }
}

function subscribe(onChange: () => void): () => void {
  window.addEventListener('storage', onChange);
  window.addEventListener(changeEvent, onChange);

  return () => {
    window.removeEventListener('storage', onChange);
    window.removeEventListener(changeEvent, onChange);
  };
}

// useCaptureFileNameTemplate は、テンプレートと、それを変える関数を返す。ほかのタブで変えても追う。
export function useCaptureFileNameTemplate(): [string, (template: string) => void] {
  const template = useSyncExternalStore(subscribe, () =>
    loadCaptureFileNameTemplate(browserStorage()),
  );

  return [
    template,
    (next) => {
      try {
        browserStorage()?.setItem(storageKey, next);
      } catch {
        // 書けなければ、次に開いたときは既定に戻るだけ。
      }
      window.dispatchEvent(new Event(changeEvent));
    },
  ];
}
