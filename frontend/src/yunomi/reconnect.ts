// ライブ視聴と録画の再生が切れたときの、つなぎ直す間隔と、切れた理由の判断 (設計書 7.2)。

// immediateRetryCooldownMs は、知らせを出さずにすぐつなぎ直してから、次にまたすぐつなぎ直せるまでの時間 (#1083)。
// つないではすぐ切れる、を繰り返すときは、知らせを出して間隔を延ばすほうに回す。
const immediateRetryCooldownMs = 30_000;

// canRetryImmediately は、前にすぐつなぎ直した時刻 (lastAt、なければ null) から、今 (now) すぐつなぎ直してよいかを返す。
export function canRetryImmediately(lastAt: number | null, now: number): boolean {
  return lastAt === null || now - lastAt >= immediateRetryCooldownMs;
}

// reconnectDelays は、つなぎ直すまでの秒。5、10、20、40 秒と延ばし、その後は 60 秒ごとに試す。
const reconnectDelays = [5, 10, 20, 40, 60];

// reconnectDelay は、attempt 回目 (0 から) につなぎ直すまでの秒を返す。
export function reconnectDelay(attempt: number): number {
  return reconnectDelays[Math.min(Math.max(attempt, 0), reconnectDelays.length - 1)]!;
}

// recentRecordingWindowMs は、視聴が切れた前後で、録画が始まったとみなす時間の幅。
// 録画は開始の少し前にチューナーを確保するので、切れた時刻の前後に幅を持たせる。
const recentRecordingWindowMs = 3 * 60_000;

// RecordingInProgress は、録画中の録画。
export type RecordingInProgress = { name: string; recordedFrom: string };

// recordingsThatTookTuner は、視聴が切れた時刻 (disconnectedAt) の前後に始まった録画を返す。
// 録画は視聴より高い優先度でチューナーを確保するので、そのときに始まった録画が、チューナーを奪ったとみなす。
export function recordingsThatTookTuner<T extends RecordingInProgress>(
  recordings: readonly T[],
  disconnectedAt: number,
): T[] {
  return recordings.filter(
    (recording) =>
      Math.abs(Date.parse(recording.recordedFrom) - disconnectedAt) <= recentRecordingWindowMs,
  );
}

// endedEarlyMarginSec は、録画の配信が終わったときに、受け取った映像の終わりが録画の終わりからこれより手前なら、途中で切れたとみなす秒。
const endedEarlyMarginSec = 10;

// endedEarly は、録画の配信が終わったとき、受け取った映像の終わり (bufferedEnd、録画の始まりからの秒) が、
// 録画の長さ (duration) に届いていなければ、途中で切れたとみなす (#1083)。tenban を止めると、配信はきれいに終わるので、
// 終わりまで流したのか、途中で切れたのかを、受け取った位置で見分ける。
export function endedEarly(bufferedEnd: number, duration: number): boolean {
  return bufferedEnd < duration - endedEarlyMarginSec;
}
