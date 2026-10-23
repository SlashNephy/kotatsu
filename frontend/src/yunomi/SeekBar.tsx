import { Slider } from '@mantine/core';
import { useEffect, useRef, useState, type PointerEvent } from 'react';
import { useTranslation } from 'react-i18next';

import { frameBefore, previewFrame, type PreviewManifest } from '../recordings/previewFrame';
import { loadManifest } from '../recordings/usePreviewSlideshow';
import classes from './SeekBar.module.css';
import { bandAt, bubbleLeft, hoverSeconds, type SeekBand } from './seekHover';

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
  // bands は、シークバーに示す区間と印 (#479)。録画が途切れた位置などを示す。
  bands?: readonly SeekBand[];
};

// SeekBar は、録画のシークバー。カーソルを乗せるか、つまみを動かしている間、その位置の時刻を、
// プレビュー画像があればそのコマと一緒に、吹き出しで出す (#755)。
export function SeekBar({
  duration,
  value,
  label,
  previewUrl,
  onChange,
  onChangeEnd,
  bands = [],
}: Props) {
  const { t } = useTranslation();
  const ref = useRef<HTMLDivElement>(null);
  // hover は、吹き出しを出す位置。offset はバーの左端からの px、seconds はその位置の秒。
  const [hover, setHover] = useState<{ offset: number; width: number; seconds: number } | null>(
    null,
  );
  // dragging は、つまみを動かしている間かどうか。その間は、カーソルがバーから外れても吹き出しを出し続ける。
  const dragging = useRef(false);
  // loaded は、読んだ preview.json と、その録画のプレビュー画像の URL。録画が変わったら、前の録画のコマの並びを使わない。
  const [loaded, setLoaded] = useState<{ url: string; manifest: PreviewManifest | null } | null>(
    null,
  );
  const manifest = loaded !== null && loaded.url === previewUrl ? loaded.manifest : null;

  // preview.json は、初めてカーソルを乗せたときに読む。見ない録画のために取りに行かない。
  const hovered = hover !== null;
  useEffect(() => {
    if (!hovered || previewUrl == null) {
      return;
    }
    let cancelled = false;
    void loadManifest(previewUrl).then((manifest) => {
      if (!cancelled) {
        setLoaded({ url: previewUrl, manifest });
      }
    });

    return () => {
      cancelled = true;
    };
  }, [hovered, previewUrl]);

  const max = Math.max(duration, 1);
  // show は、バーの中の秒 seconds の位置に、吹き出しを出す。
  const show = (seconds: number) => {
    const width = ref.current?.getBoundingClientRect().width ?? 0;
    setHover({ offset: (seconds / max) * width, width, seconds });
  };
  const track = (event: PointerEvent<HTMLDivElement>) => {
    const rect = ref.current?.getBoundingClientRect();
    if (rect === undefined) {
      return;
    }
    show(hoverSeconds(event.clientX, rect.left, rect.width, max));
  };
  // マウスでは、バーから出たら消す。タッチでは、指を離したら消す。つまみを動かしている間は消さない。
  const leave = (event: PointerEvent<HTMLDivElement>) => {
    if (dragging.current) {
      return;
    }
    if (event.pointerType !== 'mouse' || event.type === 'pointerleave') {
      setHover(null);
    }
  };

  // 印には、バーの幅の 1% (少なくとも 2 秒) 以内に乗せれば当たったとみなす。細くて、ちょうどには乗せにくいため。
  const band = hover === null ? null : bandAt(bands, hover.seconds, Math.max(max * 0.01, 2));
  const index =
    hover === null || manifest === null ? null : frameBefore(manifest.times, hover.seconds);
  const frame = manifest === null || index === null ? null : previewFrame(manifest, index);

  return (
    <div
      ref={ref}
      className={classes.root}
      onPointerMove={track}
      onPointerDown={(event) => {
        dragging.current = true;
        track(event);
      }}
      onPointerUp={leave}
      onPointerLeave={leave}
      onPointerCancel={() => {
        dragging.current = false;
        setHover(null);
      }}
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
          {band !== null && <span className={classes.label}>{band.label}</span>}
        </div>
      )}
      {/* 区間と印は、バーの上に重ねる。操作は、下のスライダーに通す。 */}
      <div className={classes.bands} aria-hidden>
        {bands.map((item, index) => (
          <div
            // 同じ位置に印が重なることもあるので (冒頭の欠けと、始まってすぐの途切れなど)、並びの番号で見分ける。
            key={index}
            className={item.to > item.from ? classes.band : classes.mark}
            style={{
              left: `${(Math.min(item.from, max) / max) * 100}%`,
              width:
                item.to > item.from
                  ? `${((Math.min(item.to, max) - item.from) / max) * 100}%`
                  : undefined,
            }}
          />
        ))}
      </div>
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
        onChange={(seconds) => {
          // つまみを動かしている間は、カーソルがバーの外にあっても、つまみの位置に吹き出しを出す。
          if (dragging.current) {
            show(seconds);
          }
          onChange(seconds);
        }}
        onChangeEnd={(seconds) => {
          // 動かし終えたら消す。カーソルがまだバーの上にあれば、次に動かしたときに出し直す。
          dragging.current = false;
          setHover(null);
          onChangeEnd(seconds);
        }}
      />
    </div>
  );
}
