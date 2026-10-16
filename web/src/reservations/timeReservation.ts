// 時刻予約の入力 (設計書 4.1)。時刻は、画面のほかの時刻と同じく、日本時間で入れる。

// maxDurationMs は、時刻予約の長さの上限。tenban と同じく 24 時間。
export const maxDurationMs = 24 * 3_600_000;

// maxNameLength は、時刻予約の名前の上限。tenban と同じく 200 文字。
export const maxNameLength = 200;

const jstOffsetMs = 9 * 3_600_000;

// toInputValue は、時刻を、datetime-local の入力の値 (日本時間の YYYY-MM-DDTHH:mm) にする。
export function toInputValue(ms: number): string {
  return new Date(ms + jstOffsetMs).toISOString().slice(0, 16);
}

// fromInputValue は、datetime-local の入力の値を、日本時間として読んだ時刻にする。読めなければ null。
export function fromInputValue(value: string): number | null {
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(value)) {
    return null;
  }
  const ms = Date.parse(`${value}:00+09:00`);

  return Number.isNaN(ms) ? null : ms;
}

// TimeRangeError は、時刻の範囲の誤り。
export type TimeRangeError = 'missing' | 'order' | 'tooLong';

// validateRange は、時刻予約の時間帯を確かめる。正しければ null を返す。
export function validateRange(start: number | null, end: number | null): TimeRangeError | null {
  if (start === null || end === null) {
    return 'missing';
  }
  if (end <= start) {
    return 'order';
  }
  if (end - start > maxDurationMs) {
    return 'tooLong';
  }

  return null;
}
