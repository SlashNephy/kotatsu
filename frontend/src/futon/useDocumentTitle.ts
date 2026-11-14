import { useEffect } from 'react';

import { splitProgramMarks } from './programMarks';

const appName = 'Kotatsu';

// titles は、今タイトルを付けている画面のタイトルを、付けた順に並べたもの。最後のものを出す。
// 離れる画面が「付ける前のタイトル」を覚えて戻すと、小さなプレイヤーに切り替わる間に開いた画面が、
// プレイヤーの番組名を覚えてしまい、その画面を離れると番組名に戻る (#1775)。
// 並べておけば、離れた画面のタイトルを外すだけで、残っている画面のタイトルに戻せる。
const titles: { title: string }[] = [];

function applyTitle(): void {
  document.title = titles.at(-1)?.title ?? appName;
}

// useDocumentTitle は、タブのタイトルを「parts | Kotatsu」(parts は「 - 」でつなぐ) にし、画面を離れたら外す。
// 番組名などに入る囲み文字は外す。null や空の部分は飛ばし、すべて空なら「Kotatsu」だけにする。
// enabled が false の間 (視聴の画面を小さなプレイヤーで描いている間など) は、タイトルを付けない。
// タイトルを付けている画面がいくつかあれば、最後に付けた画面のものを出す。後から開いた画面のタイトルを、
// 離れた画面が上書きしないためである (#1731)。どの画面も付けていなければ「Kotatsu」にする。
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
    const entry = { title };
    titles.push(entry);
    applyTitle();

    return () => {
      const index = titles.indexOf(entry);
      if (index !== -1) {
        titles.splice(index, 1);
      }
      applyTitle();
    };
  }, [title, enabled]);
}
