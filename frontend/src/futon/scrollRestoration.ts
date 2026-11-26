import type { ParsedLocation, RouterHistory } from '@tanstack/react-router';

// HistoryAction は、履歴が購読者に通知する遷移の種類 (PUSH、REPLACE、BACK、FORWARD、GO)。
type HistoryAction = Parameters<Parameters<RouterHistory['subscribe']>[0]>[0]['action']['type'];

// ルーターのスクロールの復元を、どの遷移で働かせるか (#2146)。
// ブラウザで戻ったときと進んだときは、ページと data-scroll-restoration-id を付けた要素の位置を戻す。
// ほかの画面へ新しく移ったときは、先頭から出す。同じ画面のまま、クエリを変えたり番組の詳細を開いたりしたときは、
// 位置を変えない。ルーターは、URL を置き換えたときも、履歴の項目が変わったとみなして先頭へ戻すので、それを止める。
export function createScrollRestoration(): {
  track: (history: RouterHistory) => void;
  shouldRestore: (options: { location: ParsedLocation }) => boolean;
} {
  let action: HistoryAction | null = null;
  let pathname: string | null = null;

  return {
    track: (history) => {
      history.subscribe(({ action: next }) => {
        action = next.type;
      });
    },
    shouldRestore: ({ location }) => {
      const travelled = action === 'BACK' || action === 'FORWARD' || action === 'GO';
      const samePage = location.pathname === pathname;
      pathname = location.pathname;
      action = null;

      return travelled || !samePage;
    },
  };
}
