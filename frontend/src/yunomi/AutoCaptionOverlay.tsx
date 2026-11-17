import { Select } from '@mantine/core';
import { useEffect, useEffectEvent, useState } from 'react';
import { useTranslation } from 'react-i18next';

import classes from './AutoCaptionOverlay.module.css';
import { recordingPosition, type Gap } from './position';

// CaptionTrack は、出す字幕の種類。放送の字幕 (ARIB) か、録画の音声から自動生成した字幕 (#1682)。
export type CaptionTrack = 'broadcast' | 'auto';

// AutoCaption は、自動生成した字幕の 1 つ。from と to は、途切れた分を詰めた再生の位置 (秒)。
export type AutoCaption = { from: number; to: number; text: string };

// autoCaptionPositions は、録画の時間軸の日時で受け取った字幕を、CM の区間と同じく、途切れた分を詰めた再生の位置にする。
export function autoCaptionPositions(
  recordedFrom: string,
  gaps: readonly Gap[],
  captions: readonly { readonly from: string; readonly to: string; readonly text: string }[],
): AutoCaption[] {
  return captions.map((caption) => ({
    from: recordingPosition(recordedFrom, gaps, Date.parse(caption.from)),
    to: recordingPosition(recordedFrom, gaps, Date.parse(caption.to)),
    text: caption.text,
  }));
}

// autoCaptionAt は、再生の位置 position (秒) に出す字幕を返す。なければ null。captions は始まりの順に並べたもの。
// 重なっていれば、後に始まったものを出す。
export function autoCaptionAt(
  captions: readonly AutoCaption[],
  position: number,
): AutoCaption | null {
  // position より後に始まる最初の字幕を探し、その前から遡る。
  let low = 0;
  let high = captions.length;
  while (low < high) {
    const middle = (low + high) >> 1;
    if ((captions[middle]?.from ?? Infinity) <= position) {
      low = middle + 1;
    } else {
      high = middle;
    }
  }
  for (let i = low - 1; i >= 0; i--) {
    const caption = captions[i];
    if (caption === undefined) {
      break;
    }
    if (position < caption.to) {
      return caption;
    }
    // 字幕は長くても数十秒なので、十分前に始まった字幕までは遡らない。
    if (position - caption.from > maxCaptionSeconds) {
      break;
    }
  }

  return null;
}

// maxCaptionSeconds は、autoCaptionAt が遡る、字幕の長さの上限 (秒)。whisper.cpp は 30 秒ごとに区切って読むので、それより長い字幕は出ない。
const maxCaptionSeconds = 60;

// AutoCaptionOverlay は、録画の自動生成した字幕を、映像の下に重ねて出す (#1682)。
// 字幕は録画の時間軸の日時で受け取り、途切れた分 (gaps) を詰めた再生の位置にする。
export function AutoCaptionOverlay({
  recordedFrom,
  gaps,
  captions: received,
  clock,
}: {
  recordedFrom: string;
  gaps: readonly Gap[];
  captions: readonly { readonly from: string; readonly to: string; readonly text: string }[];
  clock: () => number;
}) {
  return (
    <PositionedAutoCaptionOverlay
      captions={autoCaptionPositions(recordedFrom, gaps, received)}
      clock={clock}
    />
  );
}

// PositionedAutoCaptionOverlay は、再生の位置 (秒) で受け取った自動生成の字幕を、映像の下に重ねて出す (#1682)。
// 放送の字幕と見分けられるよう、「自動生成」の印を付ける。ライブラリのファイル (#1734) は、再生の位置で受け取るので、これを直に使う。
// 位置は clock (今の再生の位置を返す関数) から、画面の更新ごとに読み、出す字幕が変わったときだけ描き直す。
export function PositionedAutoCaptionOverlay({
  captions,
  clock,
}: {
  captions: readonly AutoCaption[];
  clock: () => number;
}) {
  const { t } = useTranslation();
  const [current, setCurrent] = useState<AutoCaption | null>(null);
  const readClock = useEffectEvent(() => clock());
  useEffect(() => {
    let frame = 0;
    const tick = () => {
      const next = autoCaptionAt(captions, readClock());
      setCurrent((previous) => (previous === next ? previous : next));
      frame = requestAnimationFrame(tick);
    };
    frame = requestAnimationFrame(tick);

    return () => cancelAnimationFrame(frame);
  }, [captions]);

  if (current === null) {
    return null;
  }

  return (
    <div className={classes.root} data-overlay="auto-captions">
      <div className={classes.box} role="status" aria-live="off">
        <span className={classes.badge}>{t('player.autoCaptions.badge')}</span>
        <span className={classes.text}>{current.text}</span>
      </div>
    </div>
  );
}

// CaptionTrackSelect は、出す字幕を、放送の字幕と、自動生成した字幕から選ぶメニュー。自動生成した字幕がなければ出さない。
export function CaptionTrackSelect({
  available,
  value,
  onChange,
}: {
  available: boolean;
  value: CaptionTrack;
  onChange: (value: CaptionTrack) => void;
}) {
  const { t } = useTranslation();
  if (!available) {
    return null;
  }

  return (
    <Select
      label={t('player.autoCaptions.track')}
      size="xs"
      // 操作バーの設定の中に置く。別のウィンドウに移っても開けるよう、メニューをプレイヤーの中に描く。
      comboboxProps={{ withinPortal: false }}
      allowDeselect={false}
      data={[
        { value: 'broadcast', label: t('player.autoCaptions.broadcast') },
        { value: 'auto', label: t('player.autoCaptions.auto') },
      ]}
      value={value}
      onChange={(next) => {
        if (next === 'broadcast' || next === 'auto') {
          onChange(next);
        }
      }}
    />
  );
}
