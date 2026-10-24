import { useState } from 'react';

// useSyncedForm は、サーバーの値 initial から作るフォームの入力を持つ (#533)。
// 編集の画面は、ストアにある値ですぐに描き、裏でサーバーから読み直すので、initial は後から変わる。
// 入力を触っていない間は、新しい initial を入れ直す。触った後に入力と違う initial が届いたら、入力を残して outdated を立てる。
// 入力と同じ initial が届いたら (保存した結果など)、触っていない状態に戻す。same は、2 つの値が同じ内容かどうかを返す。
export function useSyncedForm<T>(initial: T, same: (a: T, b: T) => boolean) {
  const [form, setForm] = useState(initial);
  const [base, setBase] = useState(initial);
  const [touched, setTouched] = useState(false);
  const [outdated, setOutdated] = useState(false);

  // 描画の中で、前に受け取った initial と比べて入れ直す。effect で入れ直すと、古い入力で一度描いてしまう。
  if (!same(initial, base)) {
    setBase(initial);
    if (!touched || same(initial, form)) {
      setForm(initial);
      setTouched(false);
      setOutdated(false);
    } else {
      setOutdated(true);
    }
  }

  return {
    form,
    // outdated は、入力を触った後に、サーバーの値が変わったかどうか。
    outdated,
    set: (patch: Partial<T>) => {
      setTouched(true);
      setForm((prev) => ({ ...prev, ...patch }));
    },
    // reload は、入力を捨てて、サーバーの値を入れ直す。
    reload: () => {
      setForm(initial);
      setTouched(false);
      setOutdated(false);
    },
  };
}
