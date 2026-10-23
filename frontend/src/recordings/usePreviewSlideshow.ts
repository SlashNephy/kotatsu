import { useReducedMotion } from '@mantine/hooks';
import { useEffect, useState } from 'react';

import {
  manifestUrl,
  previewFrame,
  slideshowFrames,
  type PreviewFrame,
  type PreviewManifest,
} from './previewFrame';

// slideCount は、スライドショーで見せるコマの数。録画全体から等間隔に選ぶ。
const slideCount = 16;
// slideInterval は、コマを切り替える間隔 (ミリ秒)。
const slideInterval = 1200;

// manifests は、読んだ preview.json。同じ録画に何度ホバーしても、取り直さない。読めなければ null。
const manifests = new Map<string, Promise<PreviewManifest | null>>();

export function loadManifest(previewUrl: string): Promise<PreviewManifest | null> {
  let manifest = manifests.get(previewUrl);
  if (manifest === undefined) {
    manifest = fetch(manifestUrl(previewUrl), { credentials: 'same-origin' })
      .then((response) => (response.ok ? (response.json() as Promise<PreviewManifest>) : null))
      .catch(() => null);
    manifests.set(previewUrl, manifest);
  }

  return manifest;
}

// usePreviewSlideshow は、active の間、プレビュー画像のコマを順に切り替える。いま見せるコマを返す。
// preview.json は、初めて active になったときに読む。プレビュー画像がないか、動きを減らす設定なら、null を返し続ける。
export function usePreviewSlideshow(
  previewUrl: string | null | undefined,
  active: boolean,
): PreviewFrame | null {
  const reducedMotion = useReducedMotion();
  const [frame, setFrame] = useState<PreviewFrame | null>(null);

  useEffect(() => {
    if (!active || previewUrl == null || reducedMotion) {
      setFrame(null);
      return;
    }
    let cancelled = false;
    let timer = 0;
    void loadManifest(previewUrl).then((manifest) => {
      if (cancelled || manifest === null) {
        return;
      }
      const frames = slideshowFrames(manifest.times.length, slideCount);
      // コマがなければ何も出さず、1 コマだけなら切り替えない。
      if (frames.length <= 1) {
        setFrame(frames[0] === undefined ? null : previewFrame(manifest, frames[0]));
        return;
      }
      let step = 0;
      const show = () => {
        setFrame(previewFrame(manifest, frames[step % frames.length]!));
        step += 1;
      };
      show();
      timer = window.setInterval(show, slideInterval);
    });

    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [active, previewUrl, reducedMotion]);

  return frame;
}
