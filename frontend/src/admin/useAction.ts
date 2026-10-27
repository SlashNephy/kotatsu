import { useRef, useState } from 'react';

// Action は、管理の操作を行う関数。onError は、操作が失敗したときに呼ぶ。
export type Action = (operation: () => Promise<void>, onError: (error: unknown) => void) => void;

// useAction は、本人確認を求めない管理の操作を行う (設計書 9.5)。
// 続けて押されても、同じ操作を重ねて送らないよう、操作の途中は次の操作を受け付けない。
// busy は、操作の途中かどうか。ボタンを押せなくするのに使う。
export function useAction(): { run: Action; busy: boolean } {
  // 押した直後の 2 回目の押下も止めるため、描画を待たない ref で持つ。
  const running = useRef(false);
  const [busy, setBusy] = useState(false);

  const run: Action = (operation, onError) => {
    if (running.current) {
      return;
    }
    running.current = true;
    setBusy(true);
    operation()
      .catch(onError)
      .finally(() => {
        running.current = false;
        setBusy(false);
      });
  };

  return { run, busy };
}
