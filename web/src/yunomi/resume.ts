// 録画を、前回の続きから再生する位置を決める (設計書 7.5)。

// minResume は、続きから再生する、保存した位置の下限 (秒)。少し見ただけなら、最初から再生する。
const minResume = 5;
// rewind は、続きから再生するとき、保存した位置から戻す長さ (秒)。止めた直前の場面から見直せるようにする。
const rewind = 5;
// endMargin は、終わりの近く (秒)。終わりの近くまで見ていたら、見終えたとみなして、最初から再生する。
export const endMargin = 10;

// resumePosition は、保存した再生位置 savedMs (ミリ秒) から、再生を始める位置 (秒) を返す。
// 保存した位置から rewind 秒戻して始める。戻した位置が minResume より前なら、最初から再生する。
export function resumePosition(savedMs: number | null, duration: number): number {
  if (savedMs === null) {
    return 0;
  }
  const seconds = savedMs / 1000;
  if (seconds > duration - endMargin) {
    return 0;
  }
  const start = seconds - rewind;
  if (start < minResume) {
    return 0;
  }

  return start;
}
