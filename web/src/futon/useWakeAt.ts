import { useEffect, useRef, useState } from 'react';

// minWakeDelayMs は、呼ぶまでに最低限あける時間。時計がずれていて、過ぎた時刻を渡され続けても、呼び続けないようにする。
const minWakeDelayMs = 1000;
// maxTimerMs は、setTimeout に渡せる最大の時間。超えると、すぐに呼ばれてしまう。
const maxTimerMs = 2 ** 31 - 1;

// useWakeAt は、at (エポックからのミリ秒) になったら onWake を 1 回呼ぶ。at が null なら呼ばない (#866)。
// 端末がスリープしている間は setTimeout の時間が進まないことがあるので、画面が見えるようになったときにも時刻を確かめる。
export function useWakeAt(at: number | null, onWake: () => void): void {
  const callback = useRef(onWake);
  useEffect(() => {
    callback.current = onWake;
  }, [onWake]);

  useEffect(() => {
    if (at === null) {
      return;
    }
    let done = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const wake = () => {
      if (done) {
        return;
      }
      done = true;
      clearTimeout(timer);
      callback.current();
    };
    // 遠い時刻は、上限まで待ってから、残りを待ち直す。
    const schedule = () => {
      const delay = at - Date.now();
      if (delay > maxTimerMs) {
        timer = setTimeout(schedule, maxTimerMs);
      } else {
        timer = setTimeout(wake, Math.max(delay, minWakeDelayMs));
      }
    };
    schedule();
    const onVisible = () => {
      if (document.visibilityState === 'visible' && Date.now() >= at) {
        wake();
      }
    };
    document.addEventListener('visibilitychange', onVisible);

    return () => {
      done = true;
      clearTimeout(timer);
      document.removeEventListener('visibilitychange', onVisible);
    };
  }, [at]);
}

// useNow は、今の時刻を返し、stepMs の区切りごとに進める。時刻の線など、時刻だけで描くものに使う (#866)。
export function useNow(stepMs: number): number {
  const [now, setNow] = useState(Date.now);
  useWakeAt(Math.floor(now / stepMs) * stepMs + stepMs, () => setNow(Date.now()));

  return now;
}

// nextWakeAt は、times (ISO 8601) のうち now より後で最も早い時刻を返す。maxWaitMs より先なら、now + maxWaitMs を返す。
// 放送中の番組の終わりなど、画面の中身が変わる時刻を選ぶのに使う。番組表にない番組が始まるのにも気づけるよう、上限を付ける (#866)。
export function nextWakeAt(times: Iterable<string>, now: number, maxWaitMs: number): number {
  let next = now + maxWaitMs;
  for (const time of times) {
    const at = Date.parse(time);
    if (at > now && at < next) {
      next = at;
    }
  }

  return next;
}
