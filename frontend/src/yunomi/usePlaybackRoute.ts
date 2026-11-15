import { useState } from 'react';

import {
  autoQuality,
  capRoute,
  decodeStepDown,
  ladder,
  loadDecodeCap,
  lowerDecodeCap,
  routeQuality,
} from './autoQuality';
import { detectCapabilities } from './capabilities';
import { loadQuality, saveQuality } from './quality';
import {
  chooseRoute,
  qualities,
  type PlaybackRoute,
  type Transcoding,
  type VideoFormat,
} from './route';
import { browserStorage } from './volume';

// usePlaybackRoute は、端末の能力と kyusu の有無と放送の映像の種類から再生の経路を決め、利用者が画質を選べるようにする (設計書 7.1)。
// 選んだ画質は端末ごとに覚え、次に開いたときも使う。「自動」(覚えていないときも) なら、デコードが追いつかないときに段を下げる (#1513)。
export function usePlaybackRoute(
  transcoding: Transcoding,
  format: VideoFormat,
): {
  route: PlaybackRoute;
  // choices は、画質のメニューに出す選択肢。選べる画質が 2 つ以上あれば、先頭に「自動」を置く。
  choices: string[];
  // quality は、メニューで選んでいるもの。「自動」なら autoQuality。再生できないときは null。
  quality: string | null;
  // current は、今の経路を表す画質の名前。再生できないときは null。
  current: string | null;
  // currentCodec は、今の経路の映像のコーデック (kyusu の経路のみ。H264 か HEVC)。
  currentCodec: string | null;
  // automatic は、画質を自動で上げ下げしてよいか (利用者が手で選んでいないか)。
  automatic: boolean;
  // lowerAvailable は、今の段より下の段を選べるか。手で選んだ画質が重いときに「自動」を勧めるかに使う。
  lowerAvailable: boolean;
  setQuality: (quality: string) => void;
  // stepDownForDecode は、デコードが追いつかないときに 1 段下げ、新しい段の名前を返す。
  // その段より上へは、タブを開いている間は上げない。手で選んでいるときと、下げる段が無いときは null。
  stepDownForDecode: () => string | null;
} {
  // 端末の能力は、画面を開いている間は変わらないので、最初に一度だけ調べる。
  const [capabilities] = useState(() => detectCapabilities());
  const [requested, setRequested] = useState(() => loadQuality(browserStorage()));
  const [cap, setCap] = useState(loadDecodeCap);
  const available = qualities(capabilities, transcoding, format);
  const rungs = ladder(available, transcoding.presets);
  const manual =
    requested !== undefined && requested !== autoQuality && available.includes(requested);
  const route = manual
    ? chooseRoute(capabilities, transcoding, requested, format)
    : capRoute(chooseRoute(capabilities, transcoding, undefined, format), cap, rungs);
  const current = routeQuality(route);
  const preset =
    route.kind === 'kyusu'
      ? transcoding.presets.find((candidate) => candidate.name === route.preset)
      : undefined;

  return {
    route,
    choices: available.length >= 2 ? [autoQuality, ...available] : available,
    quality: current === null ? null : manual || available.length < 2 ? current : autoQuality,
    current,
    currentCodec: preset?.codec ?? null,
    automatic: !manual,
    lowerAvailable:
      available.length >= 2 && current !== null && decodeStepDown(current, rungs) !== null,
    setQuality: (quality) => {
      setRequested(quality);
      saveQuality(browserStorage(), quality);
    },
    stepDownForDecode: () => {
      if (manual || current === null) {
        return null;
      }
      const next = decodeStepDown(current, rungs);
      if (next === null) {
        return null;
      }
      lowerDecodeCap(next.height);
      setCap(loadDecodeCap());

      return next.name;
    },
  };
}
