// 字幕の履歴 (#450、設計書 7.4)。出た字幕の文を時刻と一緒に残し、読み返したり、録画ではその位置へ移ったりできるようにする。

// maxCaptions は、履歴に残す字幕の数の上限。
export const maxCaptions = 100;

// CaptionEntry は、履歴の 1 件。at は、ライブでは出た時刻 (Unix ミリ秒)、録画では録画の始まりからの位置 (秒)。
// key は、履歴の中で行を見分ける番号。
export type CaptionEntry = { key: number; at: number; text: string };

// normalizeCaption は、字幕の文の前後の空白を除き、行の間の空白をそろえる。
export function normalizeCaption(text: string): string {
  return text
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line !== '')
    .join('\n');
}

// appendLiveCaption は、ライブで出た字幕を末尾に足す。空の字幕と、直前と同じ文は足さない。上限を超えたら、古いものから捨てる。
export function appendLiveCaption(
  list: readonly CaptionEntry[],
  at: number,
  raw: string,
): readonly CaptionEntry[] {
  const text = normalizeCaption(raw);
  if (text === '' || list.at(-1)?.text === text) {
    return list;
  }
  const key = (list.at(-1)?.key ?? -1) + 1;
  const next = [...list, { key, at, text }];

  return next.length > maxCaptions ? next.slice(next.length - maxCaptions) : next;
}

// sameWindow は、録画で、同じ字幕を出し直したとみなす位置の差 (秒)。シークで戻ると、同じ字幕がもう一度出る。
const sameWindow = 2;

// insertRecordedCaption は、録画で出た字幕を、位置の順に差し込む。空の字幕と、近い位置にある同じ文は足さない。
// 上限を超えたら、差し込んだ位置から最も遠いものを捨てる。見ている辺りの字幕を残すためである。
export function insertRecordedCaption(
  list: readonly CaptionEntry[],
  at: number,
  raw: string,
): readonly CaptionEntry[] {
  const text = normalizeCaption(raw);
  if (
    text === '' ||
    list.some((entry) => entry.text === text && Math.abs(entry.at - at) <= sameWindow)
  ) {
    return list;
  }
  const key = list.reduce((max, entry) => Math.max(max, entry.key), -1) + 1;
  const next = [...list, { key, at, text }].sort((a, b) => a.at - b.at);
  if (next.length <= maxCaptions) {
    return next;
  }
  const farthest =
    Math.abs(next[0]!.at - at) >= Math.abs(next.at(-1)!.at - at) ? 0 : next.length - 1;

  return next.filter((_, index) => index !== farthest);
}
