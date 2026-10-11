import type { AudioTracks } from '@mpeg2toh264/player';
import type { KyusuAudio } from './route';

// AudioChoice は、音声のメニューの選択肢。音声の ES を選ぶものと、デュアルモノの主音声と副音声を選ぶものがある (設計書 7.5)。
export type AudioChoice =
  | { value: string; kind: 'stream'; pid: number; index: number; language: string | null }
  | { value: string; kind: 'dualMono'; sub: boolean; language: string | null };

// audioChoices は、番組の音声から、音声のメニューの選択肢を返す。選べるものがなければ、空にする。
// 二か国語放送は、音声の ES を 2 つ持つ場合と、1 つの ES がデュアルモノの場合がある。
export function audioChoices(tracks: AudioTracks | null): AudioChoice[] {
  if (tracks === null) {
    return [];
  }
  const choices: AudioChoice[] = [];
  if (tracks.available.length > 1) {
    tracks.available.forEach((stream, index) => {
      choices.push({
        value: `stream:${stream.pid}`,
        kind: 'stream',
        pid: stream.pid,
        index,
        language: stream.languages[0] ?? null,
      });
    });
  }
  if (tracks.dualMono) {
    const languages =
      tracks.available.find((stream) => stream.pid === tracks.current)?.languages ?? [];
    choices.push(
      { value: 'dualMono:main', kind: 'dualMono', sub: false, language: languages[0] ?? null },
      { value: 'dualMono:sub', kind: 'dualMono', sub: true, language: languages[1] ?? null },
    );
  }

  return choices;
}

// currentAudioChoice は、今の音声を表す選択肢の値を返す。
export function currentAudioChoice(tracks: AudioTracks | null): string | null {
  if (tracks === null) {
    return null;
  }
  if (tracks.dualMono) {
    return tracks.dualMonoSub ? 'dualMono:sub' : 'dualMono:main';
  }

  return tracks.current === null ? null : `stream:${tracks.current}`;
}

// audioSlot は、ブラウザの中で変換する経路の音声の選択肢を、kyusu の音声の番号にする。
// kyusu では、tsreadex がデュアルモノの副音声と 2 つ目以降の音声の ES を、第 2 音声にする。
export function audioSlot(choice: AudioChoice): KyusuAudio {
  if (choice.kind === 'dualMono') {
    return choice.sub ? 2 : 1;
  }

  return choice.index === 0 ? 1 : 2;
}

// choiceForSlot は、kyusu の音声の番号に当たる、ブラウザの中で変換する経路の選択肢の値を返す。
// デュアルモノなら主音声か副音声を、そうでなければ音声の ES を選ぶ。当たるものがなければ null を返す。
export function choiceForSlot(choices: AudioChoice[], slot: KyusuAudio): string | null {
  const dualMono = choices.filter((choice) => choice.kind === 'dualMono');
  const candidates = dualMono.length > 0 ? dualMono : choices;

  return candidates.find((choice) => audioSlot(choice) === slot)?.value ?? null;
}

// ProgramAudio は、EPG から読んだ番組の音声 (GraphQL の ProgramAudio)。主音声が先に並ぶ。
export type ProgramAudio = {
  readonly componentType: number;
  readonly languages: ReadonlyArray<string>;
};

// dualMonoComponentType は、デュアルモノ (1/0+1/0) の component_type。
const dualMonoComponentType = 2;

// KyusuAudioChoice は、kyusu の経路の音声のメニューの選択肢。
// kind は、デュアルモノの主音声と副音声 (dualMono)、音声の ES (stream)、構成が分からないとき (unknown) のどれか。
export type KyusuAudioChoice = {
  slot: KyusuAudio;
  kind: 'dualMono' | 'stream' | 'unknown';
  language: string | null;
};

// kyusuAudioChoices は、番組の音声の構成から、kyusu の経路の音声のメニューの選択肢を返す。第 2 音声がなければ、空にする。
// 構成が分からないとき (EPG にない番組や、構成を残す前の録画) は、第 1 音声と第 2 音声の 2 つを出す。
// tsreadex は、主音声がデュアルモノなら右 (副音声) を、そうでなければ 2 つ目の音声の ES を、第 2 音声にする。
export function kyusuAudioChoices(audios: ReadonlyArray<ProgramAudio> | null): KyusuAudioChoice[] {
  const [main, second] = audios ?? [];
  if (main === undefined) {
    return [
      { slot: 1, kind: 'unknown', language: null },
      { slot: 2, kind: 'unknown', language: null },
    ];
  }
  if (main.componentType === dualMonoComponentType) {
    return [
      { slot: 1, kind: 'dualMono', language: main.languages[0] ?? null },
      { slot: 2, kind: 'dualMono', language: main.languages[1] ?? null },
    ];
  }
  if (second === undefined) {
    return [];
  }

  return [
    { slot: 1, kind: 'stream', language: main.languages[0] ?? null },
    { slot: 2, kind: 'stream', language: second.languages[0] ?? null },
  ];
}
