import { Slider } from '@mantine/core';
import { useEffect, useRef, useState, type PointerEvent } from 'react';
import { useTranslation } from 'react-i18next';

import { frameBefore, previewFrame, type PreviewManifest } from '../recordings/previewFrame';
import { loadManifest } from '../recordings/usePreviewSlideshow';
import classes from './SeekBar.module.css';
import { bubbleLeft, hoverSeconds } from './seekHover';

// bubbleWidth は、吹き出しの幅 (px)。プレビュー画像の 1 コマ (16:9) をこの幅で見せる。
const bubbleWidth = 160;

type Props = {
  // duration は、録画の長さ (秒)。
  duration: number;
  // value は、今の再生位置 (秒)。
  value: number;
  // label は、秒を、吹き出しに出す文にする。
  label: (seconds: number) => string;
  // previewUrl は、シークバー用のプレビュー画像。なければ、吹き出しには文だけを出す。
  previewUrl: string | null | undefined;
  onChange: (seconds: number) => void;
  onChangeEnd: (seconds: number) => void;
};

// SeekBar は、録画のシークバー。カーソルを乗せるか、つまみを動かしている間、その位置の時刻を、
// プレビュー画像があればそのコマと一緒に、吹き出しで出す (#755)。
export function SeekBar({ duration, value, label, previewUrl, onChange, onChangeEnd }: Props) {
  const { t } = useTranslation();
  const ref = useRef<HTMLDivElement>(null);
  // hover は、吹き出しを出す位置。offset はバーの左端からの px、seconds はその位置の秒。
  const [hover, setHover] = useState<{ offset: number; width: number; seconds: number } | null>(
    null,
  );
  const [manifest, setManifest] = useState<PreviewManifest | null>(null);

  // preview.json は、初めてカーソルを乗せたときに読む。見ない録画のために取りに行かない。
  const hovered = hover !== null;
  useEffect(() => {
    if (!hovered || previewUrl == null) {
      return;
    }
    let cancelled = false;
    void loadManifest(previewUrl).then((loaded) => {
      if (!cancelled) {
        setManifest(loaded);
      }
    });

    return () => {
      cancelled = true;
    };
  }, [hovered, previewUrl]);

  const max = Math.max(duration, 1);
  const track = (event: PointerEvent<HTMLDivElement>) => {
    const rect = ref.current?.getBoundingClientRect();
    if (rect === undefined) {
      return;
    }
    setHover({
      offset: event.clientX - rect.left,
      width: rect.width,
      seconds: hoverSeconds(event.clientX, rect.left, rect.width, max),
    });
  };
  // タッチでは、指を離したら吹き出しを消す。マウスでは、バーから出たら消す。
  const leave = (event: PointerEvent<HTMLDivElement>) => {
    if (event.pointerType !== 'mouse' || event.type === 'pointerleave') {
      setHover(null);
    }
  };

  const index =
    hover === null || manifest === null ? null : frameBefore(manifest.times, hover.seconds);
  const frame = manifest === null || index === null ? null : previewFrame(manifest, index);

  return (
    <div
      ref={ref}
      className={classes.root}
      onPointerMove={track}
      onPointerDown={track}
      onPointerUp={leave}
      onPointerLeave={leave}
      onPointerCancel={() => setHover(null)}
    >
      {hover !== null && (
        <div
          className={classes.bubble}
          style={{ left: bubbleLeft(hover.offset, hover.width, bubbleWidth), width: bubbleWidth }}
          aria-hidden
        >
          {frame !== null && previewUrl != null && (
            <div
              className={classes.frame}
              style={{
                backgroundImage: `url(${JSON.stringify(previewUrl)})`,
                backgroundSize: frame.backgroundSize,
                backgroundPosition: frame.backgroundPosition,
              }}
            />
          )}
          <span className={classes.label}>{label(hover.seconds)}</span>
        </div>
      )}
      <Slider
        aria-label={t('player.seek')}
        size={4}
        color="mikan"
        min={0}
        max={max}
        step={1}
        value={Math.min(value, duration)}
        // 位置は吹き出しで出すので、つまみの上のラベルは出さない。キーボードで動かしたときは、操作バーの時刻で分かる。
        label={null}
        onChange={onChange}
        onChangeEnd={onChangeEnd}
      />
    </div>
  );
}
