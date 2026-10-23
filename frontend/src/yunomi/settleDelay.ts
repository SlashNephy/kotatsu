import { useEffect, useState } from 'react';

// データ放送の文書が見えるか隠れるかを、少し遅らせて反映する (#445)。文書が移る途中で、隠れてはすぐ見える、を
// 繰り返すことがあり、そのたびに映像を移し直すと、ちらつくためである。

// settleMs は、変化を反映するまで待つ時間。その間に元に戻れば、何もしない。
export const settleMs = 300;

// Settler は、値の変化を、settleMs だけ落ち着くのを待ってから apply に渡す。
export type Settler<T> = {
  // set は、新しい値を受け取る。settleMs の間に変わらなければ、apply に渡す。反映した値に戻れば、待つのをやめる。
  set: (value: T) => void;
  // now は、待たずにすぐ反映する。待っていた変化は捨てる。
  now: (value: T) => void;
  cancel: () => void;
};

export function createSettler<T>(
  initial: T,
  apply: (value: T) => void,
  delay = settleMs,
): Settler<T> {
  let applied = initial;
  let timer: ReturnType<typeof setTimeout> | null = null;
  const cancel = () => {
    if (timer !== null) {
      clearTimeout(timer);
      timer = null;
    }
  };

  return {
    set: (value) => {
      cancel();
      if (value === applied) {
        return;
      }
      timer = setTimeout(() => {
        timer = null;
        applied = value;
        apply(value);
      }, delay);
    },
    now: (value) => {
      cancel();
      applied = value;
      apply(value);
    },
    cancel,
  };
}

// useSettledFalse は、value が false になるのを、settleMs だけ遅らせて返す。true になるのは、すぐに返す。
// 「受信中」の知らせを、文書が移る途中で消えては出る、を繰り返さないようにする。
export function useSettledFalse(value: boolean, delay = settleMs): boolean {
  const [settled, setSettled] = useState(value);
  useEffect(() => {
    if (value) {
      setSettled(true);

      return;
    }
    const timer = setTimeout(() => setSettled(false), delay);

    return () => clearTimeout(timer);
  }, [value, delay]);

  return value || settled;
}
