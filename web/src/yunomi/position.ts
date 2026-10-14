// 録画の再生位置の計算。シークは、新しい開始位置で読み直すので、video の時刻だけでは録画の中の位置が分からない。

// playbackPosition は、録画の始まりからの再生位置 (秒) を返す。
// offset は今の読み込みを始めた位置、base は読み込み直後の video の時刻、current は今の video の時刻。
export function playbackPosition(offset: number, base: number, current: number): number {
  return offset + Math.max(current - base, 0);
}

// formatDuration は、秒を H:MM:SS か M:SS で表す。
export function formatDuration(seconds: number): string {
  const total = Math.max(Math.floor(seconds), 0);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  const pad = (n: number) => String(n).padStart(2, '0');

  return h > 0 ? `${h}:${pad(m)}:${pad(s)}` : `${m}:${pad(s)}`;
}

// Gap は、録画の途切れた区間。区間の間は、ファイルに何も書いていない。
export type Gap = { from: string; to: string };

// broadcastTime は、録画の始まりからの再生位置 (秒) に当たる放送時刻 (Unix ミリ秒) を返す。
// ファイルには途切れた区間の分の映像がないので、その位置より前に始まった区間の長さを足す。
export function broadcastTime(recordedFrom: string, gaps: readonly Gap[], seconds: number): number {
  let at = Date.parse(recordedFrom) + Math.max(seconds, 0) * 1000;
  const sorted = [...gaps].sort((a, b) => Date.parse(a.from) - Date.parse(b.from));
  for (const gap of sorted) {
    const from = Date.parse(gap.from);
    if (from > at) {
      break;
    }
    at += Math.max(Date.parse(gap.to) - from, 0);
  }

  return at;
}

const clockFormat = new Intl.DateTimeFormat('ja-JP', {
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hourCycle: 'h23',
  timeZone: 'Asia/Tokyo',
});

// formatClock は、日時を、日本時間の HH:MM:SS で表す。
export function formatClock(ms: number): string {
  return clockFormat.format(ms);
}
