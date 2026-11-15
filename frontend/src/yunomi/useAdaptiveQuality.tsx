import { useEffect, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

import { PlayerNotice } from './PlayerControls';
import { useVideoLag, type VideoLag } from './useVideoLag';

// reasonNoticeMs は、段を動かした理由を映像の上に出しておく時間 (理由の表示時間)。
export const reasonNoticeMs = 5_000;
// hintClearMs は、手で選んだ画質が重い旨の表示を、その状態が起きなくなってから消すまでの時間 (表示を消すまでの時間)。
export const hintClearMs = 30_000;

type Notice = { kind: 'down'; name: string; at: number } | { kind: 'manual'; at: number };

// Playback は、usePlaybackRoute のうち、段を下げるのに使う部分。
type Playback = { automatic: boolean; stepDownForDecode: () => string | null };

// useAdaptiveQuality は、映像だけの遅れを飛び直しで直し (どの画面でも)、stepDown なら、デコードが追いつかない端末で段を下げる (#1513)。
// 段を下げるのは、ライブと追っかけだけ。画質が「自動」なら 1 段下げて理由を出し、手で選んでいれば動かさずに「自動」を勧める。
// notice は、映像の上に重ねる知らせ。
export function useAdaptiveQuality(
  video: HTMLVideoElement | null,
  playback: Playback,
  { stepDown }: { stepDown: boolean },
): { videoLag: VideoLag; notice: ReactNode } {
  const { t } = useTranslation();
  const [notice, setNotice] = useState<Notice | null>(null);
  const videoLag = useVideoLag(
    video,
    stepDown
      ? {
          onHeavy: () => {
            if (!playback.automatic) {
              setNotice({ kind: 'manual', at: Date.now() });

              return;
            }
            const next = playback.stepDownForDecode();
            if (next !== null) {
              setNotice({ kind: 'down', name: next, at: Date.now() });
            }
          },
        }
      : {},
  );

  // 同じ状態が続けば、知らせを出し直して、消すまでの時間を延ばす。
  useEffect(() => {
    if (notice === null) {
      return;
    }
    const timer = window.setTimeout(
      () => setNotice(null),
      notice.kind === 'down' ? reasonNoticeMs : hintClearMs,
    );

    return () => window.clearTimeout(timer);
  }, [notice]);

  return {
    videoLag,
    notice:
      notice === null ? null : (
        <PlayerNotice>
          {notice.kind === 'down'
            ? t('player.qualityStepDownDecode', { name: notice.name })
            : t('player.qualityHeavyManual')}
        </PlayerNotice>
      ),
  };
}
