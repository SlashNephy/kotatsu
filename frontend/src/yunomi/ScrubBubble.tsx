import { useEffect, useState } from 'react';

import { frameBefore, previewFrame, type PreviewManifest } from '../recordings/previewFrame';
import { loadManifest } from '../recordings/usePreviewSlideshow';
import classes from './ScrubBubble.module.css';

type Props = {
  // seconds は、行き先の秒。
  seconds: number;
  // label は、シークバーの吹き出しと同じ、行き先の経過時間と放送時刻の文。
  label: string;
  // previewUrl は、シークバー用のプレビュー画像。なければ、文だけを出す。
  previewUrl: string | null | undefined;
};

// ScrubBubble は、映像を長押ししたまま滑らせている間 (#1601)、行き先を、シークバーにつまみを乗せたときと同じく、
// 経過時間、放送時刻、プレビュー画像で、映像の上に出す。操作バーは隠しているので、シークバーの吹き出しの代わりにする。
export function ScrubBubble({ seconds, label, previewUrl }: Props) {
  const [loaded, setLoaded] = useState<{ url: string; manifest: PreviewManifest | null } | null>(
    null,
  );
  useEffect(() => {
    if (previewUrl == null) {
      return undefined;
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
  }, [previewUrl]);
  const manifest = loaded !== null && loaded.url === previewUrl ? loaded.manifest : null;
  const index = manifest === null ? null : frameBefore(manifest.times, seconds);
  const frame = manifest === null || index === null ? null : previewFrame(manifest, index);

  return (
    <div className={classes.bubble} aria-hidden>
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
      <span className={classes.label}>{label}</span>
    </div>
  );
}
