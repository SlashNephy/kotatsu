import type { AudioTracks } from '@mpeg2toh264/player';
import { describe, expect, test } from 'vitest';

import { audioChoices, currentAudioChoice } from './audio';

const mono = { pid: 0x110, componentTag: 0x10, dualMono: false, languages: ['jpn'] };
const commentary = { pid: 0x111, componentTag: 0x11, dualMono: false, languages: ['eng'] };
const bilingual = { pid: 0x110, componentTag: 0x10, dualMono: true, languages: ['jpn', 'eng'] };

describe('audioChoices', () => {
  test.each([
    { name: '音声を読む前', tracks: null, want: [], current: null },
    {
      name: '音声が 1 つで、デュアルモノでない',
      tracks: { available: [mono], current: 0x110, dualMono: false, dualMonoSub: false },
      want: [],
      current: 'stream:272',
    },
    {
      name: '音声の ES が 2 つある',
      tracks: {
        available: [mono, commentary],
        current: 0x111,
        dualMono: false,
        dualMonoSub: false,
      },
      want: ['stream:272', 'stream:273'],
      current: 'stream:273',
    },
    {
      name: 'デュアルモノの副音声を聞いている',
      tracks: { available: [bilingual], current: 0x110, dualMono: true, dualMonoSub: true },
      want: ['dualMono:main', 'dualMono:sub'],
      current: 'dualMono:sub',
    },
  ])('$name', ({ tracks, want, current }) => {
    expect(audioChoices(tracks as AudioTracks | null).map((choice) => choice.value)).toEqual(want);
    expect(currentAudioChoice(tracks as AudioTracks | null)).toBe(current);
  });

  test('デュアルモノの主音声と副音声に、記述子の言語を付ける', () => {
    const tracks: AudioTracks = {
      available: [bilingual],
      current: 0x110,
      dualMono: true,
      dualMonoSub: false,
    };

    expect(audioChoices(tracks).map((choice) => choice.language)).toEqual(['jpn', 'eng']);
  });
});
