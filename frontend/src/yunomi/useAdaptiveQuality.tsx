import { useEffect, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

import { PlayerNotice } from './PlayerControls';
import type { FallbackReason } from './decodeJudge';
import { audioStep, type NetworkMode, type StepReason } from './networkQuality';
import { original } from './route';
import { useNetworkQuality, type NetworkPlayback, type NetworkQuality } from './useNetworkQuality';
import { useVideoLag, type VideoLag } from './useVideoLag';
import type { PlayerMedia } from './playerMedia';

// reasonNoticeMs は、段階を動かした理由を映像の上に出しておく時間 (理由の表示時間)。
export const reasonNoticeMs = 5_000;
// hintClearMs は、手で選んだ画質が重い (足りない) 旨の表示を、その状態が起きなくなってから消すまでの時間 (表示を消すまでの時間)。
export const hintClearMs = 30_000;

type Notice =
  | { kind: 'down'; name: string; at: number }
  | { kind: 'manual'; at: number }
  | { kind: 'network'; name: string; direction: 'down' | 'up'; reason: StepReason; at: number }
  | { kind: 'networkManual'; at: number }
  | { kind: 'decodeFallback'; reason: FallbackReason; at: number };

// Playback は、usePlaybackRoute のうち、段階を動かすのに使う部分。
type Playback = NetworkPlayback & {
  lowerAvailable: boolean;
  stepDownForDecode: () => string | null;
  // abandonDecode は、ブラウザの中でデコードする経路から、ほかの経路へ移す (usePlaybackRoute)。その経路を使う画面だけが渡す。
  abandonDecode?: () => boolean;
};

export type AdaptiveOptions = {
  // stepDown は、デコードが追いつかない端末で段階を下げるか。ライブと追っかけだけ。
  stepDown: boolean;
  // network は、回線の側の判定の口。mode は上げ方 (ライブは probe、録画と追っかけは rate)。convert は、ブラウザ内で変換する経路か。
  // topBitrate は、最高画質 (オリジナル) のビットレート (bps)。分からなければ null。ends は、流れに終わりがあるか (録画を終えた録画など)。
  network: { mode: NetworkMode; convert: boolean; topBitrate: number | null; ends: boolean };
  // pictureInPicture は、別のウィンドウに出しているか。
  pictureInPicture?: boolean;
};

// useAdaptiveQuality は、映像だけの遅れを飛び直しで修正し (どの画面でも)、stepDown なら、デコードが追いつかない端末で段階を下げる (#1513)。
// どの画面でも、回線が足りなければ段階を下げ、余裕が戻れば上げる。
// 画質が「自動」なら段階を動かして理由を出し、手で選んでいれば動かさずに「自動」を勧める。
// notice は、映像の上に重ねる通知。network は、プレイヤーから届いた量を受け取る口と、再生の情報に出す様子。
export function useAdaptiveQuality(
  media: PlayerMedia | null,
  playback: Playback,
  { stepDown, network: networkOptions, pictureInPicture = false }: AdaptiveOptions,
): {
  videoLag: VideoLag;
  network: NetworkQuality;
  notice: ReactNode;
  // decodeFallback は、ブラウザの中でデコードする経路が間に合わないときに呼ぶ口。ほかの経路へ移し、理由を出す。
  decodeFallback: (reason: FallbackReason) => void;
} {
  const { t } = useTranslation();
  const [notice, setNotice] = useState<Notice | null>(null);
  // 映像だけの遅れの検出は、<video> の経路のための処理なので、<video> を渡す (playerMedia.ts の先頭)。
  const videoLag = useVideoLag(media?.video ?? null, {
    // フレームを canvas に描いている読み込み (#2287) では、飛び直さない。
    picturesOnCanvas: () => media?.canvasPictures != null,
    ...(stepDown
      ? {
          onHeavy: () => {
            if (!playback.automatic) {
              // 「自動」にしても下げる段階が無ければ、勧めない。
              if (playback.lowerAvailable) {
                setNotice({ kind: 'manual', at: Date.now() });
              }

              return;
            }
            const next = playback.stepDownForDecode();
            if (next !== null) {
              setNotice({ kind: 'down', name: next, at: Date.now() });
            }
          },
        }
      : {}),
  });

  const network = useNetworkQuality(media, playback, {
    ...networkOptions,
    videoLag,
    pictureInPicture,
    onStep: ({ to, direction, reason }) =>
      setNotice({ kind: 'network', name: to, direction, reason, at: Date.now() }),
    onShortManual: () => {
      if (playback.lowerAvailable) {
        setNotice({ kind: 'networkManual', at: Date.now() });
      }
    },
  });

  // 「自動」に切り替えたら、「自動」を勧める表示はすぐ消す。
  if ((notice?.kind === 'manual' || notice?.kind === 'networkManual') && playback.automatic) {
    setNotice(null);
  }

  // 同じ状態が続けば、通知を出し直して、消すまでの時間を延ばす。
  useEffect(() => {
    if (notice === null) {
      return;
    }
    const timer = window.setTimeout(
      () => setNotice(null),
      notice.kind === 'down' || notice.kind === 'network' || notice.kind === 'decodeFallback'
        ? reasonNoticeMs
        : hintClearMs,
    );

    return () => window.clearTimeout(timer);
  }, [notice]);

  // stepName は、通知に出す段階の名前。
  const stepName = (name: string): string =>
    name === original
      ? t('player.originalShort')
      : name === audioStep
        ? t('player.audioOnly')
        : name;
  const message = (shown: Notice): string => {
    switch (shown.kind) {
      case 'down':
        return t('player.qualityStepDownDecode', { name: shown.name });
      case 'manual':
        return t('player.qualityHeavyManual');
      case 'networkManual':
        return t('player.qualityNetworkManual');
      case 'decodeFallback':
        return shown.reason === 'audio'
          ? t('player.qualityDecodeFallbackAudio')
          : t('player.qualityDecodeFallback');
      case 'network':
        if (shown.direction === 'up') {
          return t('player.qualityStepUpNetwork', { name: stepName(shown.name) });
        }
        if (shown.reason === 'upFailed') {
          return t('player.qualityStepBackNetwork', { name: stepName(shown.name) });
        }

        return shown.name === audioStep
          ? t('player.qualityAudioOnlyNetwork')
          : t('player.qualityStepDownNetwork', { name: stepName(shown.name) });
    }
  };

  return {
    videoLag,
    network,
    notice: notice === null ? null : <PlayerNotice>{message(notice)}</PlayerNotice>,
    decodeFallback: (reason) => {
      if (playback.abandonDecode?.() === true) {
        setNotice({ kind: 'decodeFallback', reason, at: Date.now() });
      }
    },
  };
}
