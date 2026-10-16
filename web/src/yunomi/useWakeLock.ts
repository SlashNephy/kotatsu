import { useEffect } from 'react';

// useWakeLock は、active の間、Screen Wake Lock で画面を消さないようにする (設計書 7.5)。
// 再生している間だけ取り、止めたときと画面を離れたときに手放す。
// タブが隠れるとブラウザが手放すので、見えるようになったら取り直す。対応していないブラウザでは何もしない。
export function useWakeLock(active: boolean): void {
  useEffect(() => {
    if (!active || !('wakeLock' in navigator)) {
      return;
    }
    let disposed = false;
    let sentinel: WakeLockSentinel | null = null;
    const acquire = async () => {
      if (document.visibilityState !== 'visible' || (sentinel !== null && !sentinel.released)) {
        return;
      }
      try {
        const acquired = await navigator.wakeLock.request('screen');
        // 取れるのを待つ間に止めたら、すぐに手放す。
        if (disposed) {
          await acquired.release();

          return;
        }
        sentinel = acquired;
      } catch {
        // 省電力モードなどで断られても、再生は続ける。
      }
    };
    const onVisibilityChange = () => void acquire();
    void acquire();
    document.addEventListener('visibilitychange', onVisibilityChange);

    return () => {
      disposed = true;
      document.removeEventListener('visibilitychange', onVisibilityChange);
      void sentinel?.release().catch(() => {});
    };
  }, [active]);
}
