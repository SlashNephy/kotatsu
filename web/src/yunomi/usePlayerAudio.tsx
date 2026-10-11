import type { AudioTracks } from '@mpeg2toh264/player';
import { useState, type ReactNode } from 'react';

import { audioChoices, audioSlot, choiceForSlot, currentAudioChoice } from './audio';
import { AudioSelect, KyusuAudioSelect } from './QualitySelect';
import type { KyusuAudio, PlaybackRoute } from './route';

// usePlayerAudio は、聞く音声を選ぶ (設計書 7.5)。ブラウザの中で変換する経路では、番組の音声から選び、
// kyusu の経路では、第 1 音声と第 2 音声から選ぶ。どちらで選んでも、選んだ音声 (slot) を覚えておき、
// 画質を変えて経路が変わっても、同じ音声を聞けるようにする。
// kyusu の経路で音声を変えると、HLS の URL が変わって読み直すので、その前に onReload を呼ぶ。録画では、位置を保つのに使う。
export function usePlayerAudio(
  route: PlaybackRoute,
  onReload?: () => void,
): {
  // onAudio は、TsPlayer に渡す、番組の音声を受け取るもの。
  onAudio: (tracks: AudioTracks | null) => void;
  // convertAudio は、TsPlayer に渡す、聞く音声。null なら、プレイヤーに任せる。
  convertAudio: string | null;
  // kyusuAudio は、kyusu の HLS に入れる音声。
  kyusuAudio: KyusuAudio;
  menu: ReactNode;
} {
  const [tracks, setTracks] = useState<AudioTracks | null>(null);
  // choice は、ブラウザの中で変換する経路のメニューで選んだもの。kyusu の経路で選び直したら捨てる。
  const [choice, setChoice] = useState<string | null>(null);
  const [slot, setSlot] = useState<KyusuAudio>(1);
  const convertTracks = route.kind === 'convert' ? tracks : null;
  const choices = audioChoices(convertTracks);
  // 選んだものが今の番組の音声になければ (番組が変わって音声の構成が変わったときなど)、選んだ音声の番号から選び直す。
  // 第 1 音声なら、プレイヤーの既定 (主音声) のままにする。
  const current = choices.some((candidate) => candidate.value === choice) ? choice : null;
  const convertAudio = current ?? (slot === 1 ? null : choiceForSlot(choices, slot));

  let menu: ReactNode = null;
  if (route.kind === 'convert') {
    menu = (
      <AudioSelect
        choices={choices}
        value={currentAudioChoice(convertTracks)}
        onChange={(value) => {
          setChoice(value);
          const selected = choices.find((candidate) => candidate.value === value);
          if (selected !== undefined) {
            setSlot(audioSlot(selected));
          }
        }}
      />
    );
  } else if (route.kind === 'kyusu') {
    menu = (
      <KyusuAudioSelect
        value={slot}
        onChange={(next) => {
          if (next !== slot) {
            onReload?.();
          }
          setSlot(next);
          setChoice(null);
        }}
      />
    );
  }

  return { onAudio: setTracks, convertAudio, kyusuAudio: slot, menu };
}
