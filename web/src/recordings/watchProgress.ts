// 録画をどこまで見たかを、カードに出す割合にする (設計書 7.5)。

import { endMargin } from '../yunomi/resume';

// watchProgress は、保存した再生位置 savedMs (ミリ秒) を、録画の長さ duration (秒) に対する割合 (0〜1) にする。
// 再生していなければ null を返す。終わりの近くまで見ていたら、見終えたとみなして 1 にする (続きから再生しないのと同じ)。
export function watchProgress(savedMs: number | null, duration: number): number | null {
  if (savedMs === null || duration <= 0) {
    return null;
  }
  const seconds = savedMs / 1000;
  if (seconds >= duration - endMargin) {
    return 1;
  }

  return Math.min(Math.max(seconds / duration, 0), 1);
}
