import { useEffect } from 'react';

import { splitProgramMarks } from './programMarks';

const appName = 'Kotatsu';

// useDocumentTitle は、タブのタイトルを「parts - Kotatsu」にし、画面を離れたら元に戻す。
// 番組名などに入る囲み文字は外す。null や空の部分は飛ばす。
export function useDocumentTitle(parts: readonly (string | null)[]): void {
  const title = [
    ...parts.flatMap((part) => {
      const name = part === null ? '' : splitProgramMarks(part).name;

      return name === '' ? [] : [name];
    }),
    appName,
  ].join(' - ');

  useEffect(() => {
    const previous = document.title;
    document.title = title;

    return () => {
      document.title = previous;
    };
  }, [title]);
}
