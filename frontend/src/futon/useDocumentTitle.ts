import { useEffect } from 'react';

import { splitProgramMarks } from './programMarks';

const appName = 'Kotatsu';

// useDocumentTitle は、タブのタイトルを「parts | Kotatsu」(parts は「 - 」でつなぐ) にし、画面を離れたら元に戻す。
// 番組名などに入る囲み文字は外す。null や空の部分は飛ばし、すべて空なら「Kotatsu」だけにする。
export function useDocumentTitle(parts: readonly (string | null)[]): void {
  const names = parts.flatMap((part) => {
    const name = part === null ? '' : splitProgramMarks(part).name;

    return name === '' ? [] : [name];
  });
  const title = names.length === 0 ? appName : `${names.join(' - ')} | ${appName}`;

  useEffect(() => {
    const previous = document.title;
    document.title = title;

    return () => {
      document.title = previous;
    };
  }, [title]);
}
