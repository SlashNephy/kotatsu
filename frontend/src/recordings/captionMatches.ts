// 録画の検索で当たった字幕 (#2215) の、再生を始める位置の計算。

import { recordingPosition, type Gap } from '../yunomi/position';

// captionStartSeconds は、録画の時間軸の日時 from に始まる字幕を、その場面から再生するときの位置 (秒) を返す。
// 字幕の時刻は途切れた区間を含む録画の時間軸の値なので、途切れた分を詰めた再生の位置に直す。
// 再生の画面の t は秒の整数で読むので、切り捨てる。字幕の始まりより後から再生しないよう、切り上げない。
export function captionStartSeconds(
  recordedFrom: string,
  gaps: readonly Gap[],
  from: string,
): number {
  return Math.floor(recordingPosition(recordedFrom, gaps, Date.parse(from)));
}
