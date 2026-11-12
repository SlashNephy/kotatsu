import { useEffect, useRef } from 'react';

import { useLiveScrambledEvent } from '../changes/DataChanges';

// useLiveScrambled は、ライブのサービス serviceId が、スクランブルされたまま届くと決まったら onScrambled を呼ぶ (設計書 7.1、#1517)。
// 決めるのは、ストリームを中継する tenban か kyusu で、決めたらストリームを閉じるので、プレイヤーはつなぎ直すのをやめて理由を出す。
// 知らせは、接続を増やさないよう、ヘッダーで購読している viewerEvents で届く (設計書 9.1)。この画面を開く前に届いた知らせでは呼ばない。
export function useLiveScrambled(serviceId: string, onScrambled: () => void) {
  const event = useLiveScrambledEvent();
  // 開いたときに届いていた知らせは、前に見た局のものなので、扱わない。
  const seen = useRef(event);
  const callback = useRef(onScrambled);
  callback.current = onScrambled;
  useEffect(() => {
    if (event === null || event === seen.current) {
      return;
    }
    seen.current = event;
    if (event.service === serviceId) {
      callback.current();
    }
  }, [event, serviceId]);
}
