import { useRef, useState } from 'react';

import { useSaveNotice } from './saveNotice';

// SaveCancelled は、保存の前に求めた再認証を、利用者がやめたことを表す。保存できなかった知らせは出さない。
export class SaveCancelled extends Error {
  constructor() {
    super('save cancelled');
    this.name = 'SaveCancelled';
  }
}

// AutoSave は、設定を保存する operation を動かし、保存できたかを返す関数。
export type AutoSave = (operation: () => Promise<unknown>) => Promise<boolean>;

// SaveCallbacks は、保存の結果を受け取る関数。楽観的に画面へ出す mutation のように、Promise を返さない保存に渡す。
export type SaveCallbacks = {
  onSaved?: () => void;
  onFailed?: (error: unknown) => void;
};

// useAutoSave は、設定の画面で、変えた設定を保存し、結果を通知で知らせる (#1740)。
// 保存は、受け取った順に 1 つずつ送る。欄を離れて保存している途中に切り替えを押しても、捨てたり押せなくしたりせず、
// 前の保存を待ってから送る。重ねて送ると、後から届いた古い値で上書きしかねないため。
// saving は、保存の途中かどうか。
export function useAutoSave(): { save: AutoSave; saving: boolean } {
  const notice = useSaveNotice();
  // queue は、最後に受け取った保存。次の保存は、これを待ってから送る。
  const queue = useRef<Promise<unknown>>(Promise.resolve());
  const pending = useRef(0);
  const [saving, setSaving] = useState(false);

  const save: AutoSave = (operation) => {
    pending.current += 1;
    setSaving(true);
    const run = queue.current
      .then(operation)
      .then(
        () => {
          notice.saved();

          return true;
        },
        (error: unknown) => {
          if (!(error instanceof SaveCancelled)) {
            notice.failed(error);
          }

          return false;
        },
      )
      .finally(() => {
        pending.current -= 1;
        if (pending.current === 0) {
          setSaving(false);
        }
      });
    queue.current = run;

    return run;
  };

  return { save, saving };
}

// useDeviceSave は、この端末だけに覚える設定 (localStorage) を変えたときに、保存した知らせを出す関数を返す (#1740)。
// set を包み、変えたら同じ通知を出す。サーバーへの保存と違い、失敗しないので、すぐ知らせる。
export function useDeviceSave() {
  const notice = useSaveNotice();

  return <T>(set: (value: T) => void) =>
    (value: T) => {
      set(value);
      notice.saved();
    };
}
