import { useEffect, useRef, useState } from 'react';

import { usePlayerEvent, type PlayerEvents } from './playerState';

// programOsdMs は、番組の情報を映像に重ねて出しておく長さ。
export const programOsdMs = 5_000;

// useProgramOsd は、ライブ視聴で、番組の情報を映像に重ねて出すかどうかを返す (#815、設計書 7.5)。
// 選局して映像が出たときと、放送中の番組が変わったときに、programOsdMs だけ出す。局を変えると画面ごと作り直すので、
// 映像が出たら出す。ready は、映像が出たかどうか。enabled が false の間 (バッファを再生している間など) は出さない。
export function useProgramOsd(events: PlayerEvents, ready: boolean, enabled: boolean): boolean {
  // shown は、出した回数。出し直すたびに増やして、隠すまでの時間を数え直す。
  const [shown, setShown] = useState(0);
  const [visible, setVisible] = useState(false);
  // tuned は、選局して映像が出たときの分を出したかどうか。
  const [tuned, setTuned] = useState(false);
  const enabledRef = useRef(enabled);
  enabledRef.current = enabled;

  if (ready && enabled && !tuned) {
    setTuned(true);
    setVisible(true);
    setShown((n) => n + 1);
  }
  usePlayerEvent(events, 'programChanged', () => {
    if (enabledRef.current) {
      setVisible(true);
      setShown((n) => n + 1);
    }
  });

  useEffect(() => {
    if (shown === 0) {
      return;
    }
    const timer = setTimeout(() => setVisible(false), programOsdMs);

    return () => clearTimeout(timer);
  }, [shown]);

  return visible && enabled;
}
