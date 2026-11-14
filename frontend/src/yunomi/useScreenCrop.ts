import { useEffect, useState } from 'react';

import { screenCrop, type Crop } from './crop';

// useScreenCrop は、画面に映すときの切り抜きを返す (#1599)。画面いっぱいに広げる表示の比率 (fill) では、
// 全画面の間、画面の大きさを追い、端末を回して比率が変わっても覆い直す。
export function useScreenCrop(crop: Crop, fullscreen: boolean): Crop {
  const following = crop.fill === true && fullscreen;
  const [size, setSize] = useState(() => ({
    width: window.innerWidth,
    height: window.innerHeight,
  }));

  useEffect(() => {
    if (!following) {
      return undefined;
    }
    const update = () => setSize({ width: window.innerWidth, height: window.innerHeight });
    update();
    window.addEventListener('resize', update);

    return () => window.removeEventListener('resize', update);
  }, [following]);

  return screenCrop(crop, fullscreen, size.width, size.height);
}
