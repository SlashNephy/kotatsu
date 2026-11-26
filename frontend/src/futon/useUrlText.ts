import { useState } from 'react';

// 画面の URL のクエリに持つ、探す語 (#2146)。戻ったときや、共有やブックマークで開き直したときに、同じ語で探す。

// parseUrlText は、クエリの探す語を読む。空白だけの語は、探さないので破棄する。
export function parseUrlText(value: unknown): string | undefined {
  // 数字だけの語は、ルーターが数として読むので、文字列に戻す。
  const text = typeof value === 'string' || typeof value === 'number' ? String(value) : undefined;

  return text !== undefined && text.trim() !== '' ? text : undefined;
}

// useUrlText は、クエリに持つ探す語の入力を返す。urlText は今のクエリの語、write はクエリを置き換える関数。
// 打つたびにクエリを置き換えるが、空白を打った途中の値も残すよう、入力は手元に持つ。
// 上のメニューから開き直すなど、入力の外でクエリの語が変わったら、入力をそれに合わせる。
export function useUrlText(
  urlText: string | undefined,
  write: (text: string | undefined) => void,
): [string, (text: string) => void] {
  const [text, setText] = useState(urlText ?? '');
  const [synced, setSynced] = useState(urlText);
  if (urlText !== synced) {
    setSynced(urlText);
    if ((urlText ?? '') !== text && !(urlText === undefined && text.trim() === '')) {
      setText(urlText ?? '');
    }
  }
  const change = (value: string) => {
    setText(value);
    write(value.trim() === '' ? undefined : value);
  };

  return [text, change];
}
