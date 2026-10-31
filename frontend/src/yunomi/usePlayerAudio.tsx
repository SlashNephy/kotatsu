import type { AudioTracks } from '@mpeg2toh264/player';
import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

import {
  audioChoices,
  audioSlot,
  choiceForComponent,
  choiceForSlot,
  currentAudioChoice,
  kyusuAudioChoices,
  type ProgramAudio,
} from './audio';
import { preferredSlot, useAudioLanguage } from './audioLanguage';
import {
  AudioLanguageSelect,
  AudioSelect,
  audioChoiceLabel,
  KyusuAudioSelect,
  kyusuAudioChoiceLabel,
} from './QualitySelect';
import type { KyusuAudio, PlaybackRoute } from './route';

// usePlayerAudio は、聞く音声を選ぶ (設計書 7.5)。ブラウザの中で変換する経路では、番組の音声から選び、
// kyusu の経路では、番組の音声の構成 (programAudios) から、第 1 音声と第 2 音声を選ぶ。第 2 音声がない番組では、第 1 音声にする。
// どちらで選んでも、選んだ音声 (slot) を覚えておき、画質を変えて経路が変わっても、同じ音声を聞けるようにする。
// kyusu の経路で音声を変えると、HLS の URL が変わって読み直すので、その前に onReload を呼ぶ。録画では、位置を保つのに使う。
// programAudios は、EPG から読んだ番組の音声の構成。分からなければ null にする。
export function usePlayerAudio(
  route: PlaybackRoute,
  programAudios: ReadonlyArray<ProgramAudio> | null,
  onReload?: () => void,
): {
  // onAudio は、TsPlayer に渡す、番組の音声を受け取るもの。
  onAudio: (tracks: AudioTracks | null) => void;
  // convertAudio は、TsPlayer に渡す、聞く音声。null なら、プレイヤーに任せる。
  convertAudio: string | null;
  // kyusuAudio は、kyusu の HLS に入れる音声。
  kyusuAudio: KyusuAudio;
  menu: ReactNode;
  available: boolean;
  // selectComponent は、データ放送の文書が選んだ音声に切り替える (#440)。ブラウザの中で変換する経路でだけ選べる。
  // 選べたら true を返す。
  selectComponent: (componentTag: number, channelId?: number) => boolean;
  // remote は、ほかの端末から音声を選ぶためのもの (#459)。選べる音声が 2 つ以上なければ、choices は空。
  remote: {
    choices: ReadonlyArray<{ id: string; label: string }>;
    selected: string | null;
    select: (id: string) => void;
  };
} {
  const { t } = useTranslation();
  const [tracks, setTracks] = useState<AudioTracks | null>(null);
  // choice は、ブラウザの中で変換する経路のメニューで選んだもの。kyusu の経路で選び直したら捨てる。
  const [choice, setChoice] = useState<string | null>(null);
  // pickedSlot は、利用者が選んだ音声の番号。選ぶまでは null で、番組ごとに、優先する言語 (#819) の音声か、第 1 音声にする。
  const [pickedSlot, setSlot] = useState<KyusuAudio | null>(null);
  const [language] = useAudioLanguage();
  const convertTracks = route.kind === 'convert' ? tracks : null;
  const kyusuChoices = kyusuAudioChoices(programAudios);
  const slot = pickedSlot ?? preferredSlot(kyusuChoices, language) ?? 1;
  // 選んだ番号の音声がない番組では、第 1 音声を流す。選んだ番号は、次に第 2 音声がある番組のために覚えておく。
  const kyusuAudio = kyusuChoices.some((candidate) => candidate.slot === slot) ? slot : 1;
  const choices = audioChoices(convertTracks);
  // 選んだものが今の番組の音声になければ (番組が変わって音声の構成が変わったときなど)、選んだ音声の番号から選び直す。
  // 第 1 音声なら、プレイヤーの既定 (主音声) のままにする。
  const current = choices.some((candidate) => candidate.value === choice) ? choice : null;
  const convertAudio = current ?? (slot === 1 ? null : choiceForSlot(choices, slot));

  const selectConvert = (value: string) => {
    setChoice(value);
    const selected = choices.find((candidate) => candidate.value === value);
    if (selected !== undefined) {
      setSlot(audioSlot(selected));
    }
  };
  const selectKyusu = (next: KyusuAudio) => {
    if (next !== kyusuAudio) {
      onReload?.();
    }
    setSlot(next);
    setChoice(null);
  };

  let menu: ReactNode = null;
  // available は、選べる音声が 2 つ以上あるかどうか。なければ、メニューは何も出さない。
  const available =
    route.kind === 'convert'
      ? choices.length >= 2
      : route.kind === 'kyusu' && kyusuChoices.length >= 2;

  if (route.kind === 'convert') {
    menu = (
      <AudioSelect
        choices={choices}
        value={currentAudioChoice(convertTracks)}
        onChange={selectConvert}
      />
    );
  } else if (route.kind === 'kyusu') {
    menu = <KyusuAudioSelect choices={kyusuChoices} value={kyusuAudio} onChange={selectKyusu} />;
  }
  // 選べる音声があるときは、優先する言語 (#819) も、音声の選択の下で選べるようにする。
  if (available) {
    menu = (
      <>
        {menu}
        <AudioLanguageSelect />
      </>
    );
  }

  const selectComponent = (componentTag: number, channelId?: number) => {
    const value = choiceForComponent(convertTracks, componentTag, channelId);
    if (value === null) {
      return false;
    }
    setChoice(value);
    const selected = choices.find((candidate) => candidate.value === value);
    if (selected !== undefined) {
      setSlot(audioSlot(selected));
    }

    return true;
  };

  let remote: {
    choices: ReadonlyArray<{ id: string; label: string }>;
    selected: string | null;
    select: (id: string) => void;
  } = { choices: [], selected: null, select: () => {} };
  if (available && route.kind === 'convert') {
    remote = {
      choices: choices.map((choice) => ({ id: choice.value, label: audioChoiceLabel(t, choice) })),
      selected: currentAudioChoice(convertTracks),
      select: (id) => {
        if (choices.some((candidate) => candidate.value === id)) {
          selectConvert(id);
        }
      },
    };
  } else if (available && route.kind === 'kyusu') {
    remote = {
      choices: kyusuChoices.map((choice) => ({
        id: String(choice.slot),
        label: kyusuAudioChoiceLabel(t, choice),
      })),
      selected: String(kyusuAudio),
      select: (id) => {
        if (id === '1' || id === '2') {
          selectKyusu(id === '1' ? 1 : 2);
        }
      },
    };
  }

  return { onAudio: setTracks, convertAudio, kyusuAudio, menu, available, selectComponent, remote };
}
