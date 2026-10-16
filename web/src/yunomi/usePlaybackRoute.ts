import { useState } from 'react';

import { detectCapabilities } from './capabilities';
import { loadQuality, saveQuality } from './quality';
import { chooseRoute, original, qualities, type PlaybackRoute, type Transcoding } from './route';
import { browserStorage } from './volume';

// usePlaybackRoute は、端末の能力と kyusu の有無から再生の経路を決め、利用者が画質を選べるようにする (設計書 7.1)。
// 選んだ画質は端末ごとに覚え、次に開いたときも使う。
export function usePlaybackRoute(transcoding: Transcoding): {
  route: PlaybackRoute;
  // choices は、画質のメニューに出す選択肢。
  choices: string[];
  // quality は、今の経路を表す画質の名前。再生できないときは null。
  quality: string | null;
  setQuality: (quality: string) => void;
} {
  // 端末の能力は、画面を開いている間は変わらないので、最初に一度だけ調べる。
  const [capabilities] = useState(() => detectCapabilities());
  const [requested, setRequested] = useState(() => loadQuality(browserStorage()));
  const route = chooseRoute(capabilities, transcoding, requested);

  return {
    route,
    choices: qualities(capabilities, transcoding),
    quality: route.kind === 'convert' ? original : route.kind === 'kyusu' ? route.preset : null,
    setQuality: (quality) => {
      setRequested(quality);
      saveQuality(browserStorage(), quality);
    },
  };
}
