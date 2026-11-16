import { useState } from 'react';

import {
  audioOnlyHeight,
  autoQuality,
  capRoute,
  decodeStepDown,
  ladder,
  loadDecodeCap,
  loadNetworkCap,
  lowerDecodeCap,
  minCap,
  qualityHeight,
  routeQuality,
  saveNetworkCap,
} from './autoQuality';
import { detectCapabilities } from './capabilities';
import { audioOnlyBitrate, audioStep, type Step } from './networkQuality';
import { loadQuality, saveQuality } from './quality';
import {
  chooseRoute,
  original,
  qualities,
  type PlaybackRoute,
  type Transcoding,
  type VideoFormat,
} from './route';
import { browserStorage } from './volume';

// usePlaybackRoute は、端末の能力と kyusu の有無と放送の映像の種類から再生の経路を決め、利用者が画質を選べるようにする (設計書 7.1)。
// 選んだ画質は端末ごとに覚え、次に開いたときも使う。「自動」(覚えていないときも) なら、デコードが追いつかないときと、
// 回線が足りないときに段を下げ、回線の余裕が戻れば上げる (#1513)。
// inheritCap は、ほかの画面で下げた段の上限を、開いたときに引き継ぐか。段を下げる画面 (ライブ、追っかけ再生、放送中の番組の
// タイムシフト) でだけ引き継ぎ、録画を終えた録画の再生では引き継がない。開いた後に変わっても (録画を終えたなど)、経路は変えない。
// 回線のための上限は、どの画面でも引き継ぐ。
// audioOnly は、回線のために音声だけ (kyusu の映像を送らない画質) まで下げてよいか。ライブを流している間だけ。
export function usePlaybackRoute(
  transcoding: Transcoding,
  format: VideoFormat,
  { inheritCap = true, audioOnly = false }: { inheritCap?: boolean; audioOnly?: boolean } = {},
): {
  route: PlaybackRoute;
  // choices は、画質のメニューに出す選択肢。選べる画質が 2 つ以上あれば、先頭に「自動」を置く。
  choices: string[];
  // quality は、メニューで選んでいるもの。「自動」なら autoQuality。再生できないときは null。
  quality: string | null;
  // current は、今の経路を表す画質の名前。音声だけなら audioStep。再生できないときは null。
  current: string | null;
  // currentCodec は、今の経路の映像のコーデック (kyusu の経路のみ。H264 か HEVC)。
  currentCodec: string | null;
  // automatic は、画質を自動で上げ下げしてよいか (利用者が手で選んでいないか)。
  automatic: boolean;
  // lowerAvailable は、今の段より下の段を選べるか。手で選んだ画質が重いときに「自動」を勧めるかに使う。
  lowerAvailable: boolean;
  // networkLadder は、回線のために動かしてよい段を高い順に並べたもの。自動で選ぶ経路より上と、デコードが追いつかなかった段より
  // 上は除く。音声だけにしてよければ、最後に音声だけの段を置く。手で選んでいるときは、判定にだけ使うので、すべての段の表。
  networkLadder: Step[];
  setQuality: (quality: string) => void;
  // stepDownForDecode は、デコードが追いつかないときに 1 段下げ、新しい段の名前を返す。
  // その段より上へは、タブを開いている間は上げない。手で選んでいるときと、下げる段が無いときは null。
  stepDownForDecode: () => string | null;
  // setNetworkStep は、回線のために、段を name (networkLadder の名前) にする。手で選んでいるときは何もしない。
  setNetworkStep: (name: string) => void;
} {
  // 端末の能力は、画面を開いている間は変わらないので、最初に一度だけ調べる。
  const [capabilities] = useState(() => detectCapabilities());
  const [requested, setRequested] = useState(() => loadQuality(browserStorage()));
  const [cap, setCap] = useState(() => (inheritCap ? loadDecodeCap() : null));
  const [networkCap, setNetworkCap] = useState(() => loadNetworkCap());
  const available = qualities(capabilities, transcoding, format);
  const rungs = ladder(available, transcoding.presets);
  const manual =
    requested !== undefined && requested !== autoQuality && available.includes(requested);
  // kyusu の H.264 の段があれば、kyusu が動いていて、端末が HLS を再生できる。音声だけの画質も、kyusu で送る。
  const audioAvailable = audioOnly && rungs.some((rung) => rung.name !== original);
  const base = chooseRoute(capabilities, transcoding, undefined, format);
  const route = manual
    ? chooseRoute(capabilities, transcoding, requested, format)
    : capRoute(base, minCap(cap, networkCap), rungs, { audioOnly: audioAvailable });
  const current = routeQuality(route);
  const preset =
    route.kind === 'kyusu'
      ? transcoding.presets.find((candidate) => candidate.name === route.preset)
      : undefined;
  // 回線のために動かしてよい段は、自動で選ぶ経路の高さと、デコードの上限の低い方まで。
  const baseName = routeQuality(base);
  const ceiling = minCap(baseName === null ? null : qualityHeight(baseName), cap);
  const allowed = rungs.filter((rung) => ceiling === null || rung.height <= ceiling);
  const networkLadder: Step[] = (manual ? rungs : allowed).map((rung) => ({
    name: rung.name,
    bitrate: rung.bitrate,
  }));
  if (audioAvailable && !manual) {
    networkLadder.push({ name: audioStep, bitrate: audioOnlyBitrate });
  }

  return {
    route,
    choices: available.length >= 2 ? [autoQuality, ...available] : available,
    quality: current === null ? null : manual || available.length < 2 ? current : autoQuality,
    current,
    currentCodec: preset?.codec ?? null,
    automatic: !manual,
    lowerAvailable:
      available.length >= 2 && current !== null && decodeStepDown(current, rungs) !== null,
    networkLadder,
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
    setNetworkStep: (name) => {
      if (manual) {
        return;
      }
      const height = name === audioStep ? audioOnlyHeight : qualityHeight(name);
      if (height === null) {
        return;
      }
      // 動かしてよい段の一番上まで上げたら、上限を外す。
      const next = allowed[0] !== undefined && height >= allowed[0].height ? null : height;
      saveNetworkCap(next);
      setNetworkCap(next);
    },
  };
}
