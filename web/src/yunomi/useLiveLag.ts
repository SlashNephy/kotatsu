import { useEffect, useRef, useState } from 'react';

// liveReloadAfterMs は、ライブを一時停止してから、再開するときに放送中の位置から読み直すまでの時間。
// ブラウザ内での変換では、止めている間もストリームを受け取って溜めるので、長く止めるほど放送から遅れる (#449)。
export const liveReloadAfterMs = 10_000;

// useLiveLag は、ライブの再生が放送から遅れているかを返す (設計書 7.2、7.3)。
// 一時停止したら遅れているとみなし、読み直す (loadstart) まで続ける。
// reloadOnResume なら、liveReloadAfterMs より長く止めていたとき、再開したところで onReload を呼び、放送中の位置から読み直させる。
export function useLiveLag(
  video: HTMLVideoElement | null,
  { reloadOnResume, onReload }: { reloadOnResume: boolean; onReload: () => void },
): boolean {
  const [behind, setBehind] = useState(false);
  const onReloadRef = useRef(onReload);
  onReloadRef.current = onReload;

  useEffect(() => {
    if (video === null) {
      return;
    }
    let pausedAt: number | null = null;
    const pause = () => {
      // 読み終えて止まったのは、一時停止ではない。
      if (video.ended) {
        return;
      }
      pausedAt = Date.now();
      setBehind(true);
    };
    const play = () => {
      if (pausedAt === null) {
        return;
      }
      const paused = Date.now() - pausedAt;
      pausedAt = null;
      if (reloadOnResume && paused > liveReloadAfterMs) {
        onReloadRef.current();
      }
    };
    const loadStart = () => {
      pausedAt = null;
      setBehind(false);
    };
    video.addEventListener('pause', pause);
    video.addEventListener('play', play);
    video.addEventListener('loadstart', loadStart);

    return () => {
      video.removeEventListener('pause', pause);
      video.removeEventListener('play', play);
      video.removeEventListener('loadstart', loadStart);
      setBehind(false);
    };
  }, [video, reloadOnResume]);

  return behind;
}
