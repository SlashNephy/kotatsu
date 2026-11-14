import { useEffect } from 'react';

import { splitProgramMarks } from './programMarks';

const appName = 'Kotatsu';

// useDocumentTitle は、タブのタイトルを「parts | Kotatsu」(parts は「 - 」でつなぐ) にし、画面を離れたら元に戻す。
// 番組名などに入る囲み文字は外す。null や空の部分は飛ばし、すべて空なら「Kotatsu」だけにする。
// enabled が false の間 (視聴の画面を小さなプレイヤーで描いている間など) は、タイトルを変えない。
// 元に戻すのは、タイトルがまだ自分の付けたもののときだけにする。後から開いた画面が付けたタイトルを、
// 離れた画面が古いタイトルで上書きしないためである (#1731)。
export function useDocumentTitle(parts: readonly (string | null)[], enabled = true): void {
  const names = parts.flatMap((part) => {
    const name = part === null ? '' : splitProgramMarks(part).name;

    return name === '' ? [] : [name];
  });
  const title = names.length === 0 ? appName : `${names.join(' - ')} | ${appName}`;

  useEffect(() => {
    if (!enabled) {
      return;
    }
    const previous = document.title;
    document.title = title;

    return () => {
      if (document.title === title) {
        document.title = previous;
      }
    };
  }, [title, enabled]);
}
