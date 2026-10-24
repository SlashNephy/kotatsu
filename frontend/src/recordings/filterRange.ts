// 録画の一覧の、放送日の期間の絞り込み。入力は端末のローカルの日付 (YYYY-MM-DD) で、API には日時で渡す。

// broadcastRange は、from から to までの放送日 (両端を含む) を、API の since と until にする。
// until は、to の翌日の 0 時 (含まない)。空の日付は、その側を区切らない。
export function broadcastRange(from: string, to: string): { since?: string; until?: string } {
  const range: { since?: string; until?: string } = {};
  const since = localMidnight(from, 0);
  if (since !== null) {
    range.since = since.toISOString();
  }
  const until = localMidnight(to, 1);
  if (until !== null) {
    range.until = until.toISOString();
  }

  return range;
}

// localMidnight は、日付 (YYYY-MM-DD) の days 日後の、端末のローカルの 0 時を返す。日付でなければ null を返す。
function localMidnight(date: string, days: number): Date | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(date);
  if (match === null) {
    return null;
  }

  return new Date(Number(match[1]), Number(match[2]) - 1, Number(match[3]) + days);
}
