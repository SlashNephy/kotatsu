import type { AudioTracks } from '@mpeg2toh264/player';

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
