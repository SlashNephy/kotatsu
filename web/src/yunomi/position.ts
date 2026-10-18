// 録画の再生位置の計算。シークは、新しい開始位置で読み直すので、video の時刻だけでは録画の中の位置が分からない。

import { zonedFormat } from '../futon';

// playbackPosition は、録画の始まりからの再生位置 (秒) を返す。
// offset は今の読み込みを始めた位置、base は読み込み直後の video の時刻、current は今の video の時刻。
export function playbackPosition(offset: number, base: number, current: number): number {
  return offset + Math.max(current - base, 0);
}

// maxVideoJump は、変換した映像の時刻が、続く timeupdate の間に進む最も長い秒。これより飛んだら、録画の途切れた区間を越えたとみなす。
// サーバーが再生の時間で途切れた区間を詰めるときの、I フレームの間隔の上限 (5 秒) とそろえる (設計書 7.2)。
const maxVideoJump = 5;

// skipJump は、変換した映像の時刻が、前に見た時刻 last から current へ飛んでいたら、飛んだ分を base に足して返す。
// ブラウザの中で変換するプレイヤーは、途切れた区間の前後で PTS が飛ぶと、video の時刻も飛ばす。
// 再生位置は途切れた区間を含めないので、飛んだ分を位置に数えないようにする。
export function skipJump(base: number, last: number, current: number): number {
  const delta = current - last;

  return Math.abs(delta) > maxVideoJump ? base + delta : base;
}

// parseStreamStart は、録画の配信が返す、流し始めた位置のヘッダー (X-Kotatsu-Start、秒) を読む。読めなければ null。
export function parseStreamStart(value: string | null): number | null {
  if (value === null || value.trim() === '') {
    return null;
  }
  const seconds = Number(value);

  return Number.isFinite(seconds) && seconds >= 0 ? seconds : null;
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

// recordingPosition は、放送時刻 at (Unix ミリ秒) に当たる、録画の始まりからの再生位置 (秒) を返す。broadcastTime の逆。
// 途切れた区間の分の映像はファイルにないので、それより前に終わった区間の長さを引く。区間の中の時刻は、区間の始まりの位置にする。
// recordedFrom より前の区間 (遅れて録り始めたときの、冒頭の欠け) は、位置の基準の前なので引かない。
export function recordingPosition(recordedFrom: string, gaps: readonly Gap[], at: number): number {
  const start = Date.parse(recordedFrom);
  let missing = 0;
  for (const gap of gaps) {
    const from = Math.max(Date.parse(gap.from), start);
    const to = Date.parse(gap.to);
    if (from < at) {
      missing += Math.max(Math.min(to, at) - from, 0);
    }
  }

  return Math.max((at - start - missing) / 1000, 0);
}

const clockFormat = zonedFormat({
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hourCycle: 'h23',
});

// formatClock は、日時を、表示のタイムゾーンの HH:MM:SS で表す。
export function formatClock(ms: number): string {
  return clockFormat.format(ms);
}
