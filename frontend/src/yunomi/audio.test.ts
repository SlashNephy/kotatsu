import type { AudioTracks } from '@mpeg2toh264/player';
import { describe, expect, test } from 'vitest';

import {
  audioChoices,
  audioSlot,
  choiceForComponent,
  choiceForSlot,
  currentAudioChoice,
  decodedAudioTracks,
  kyusuAudioChoices,
} from './audio';

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

describe('audioSlot と choiceForSlot', () => {
  const twoStreams = audioChoices({
    available: [mono, commentary],
    current: 0x110,
    dualMono: false,
    dualMonoSub: false,
  });
  const dualMono = audioChoices({
    available: [bilingual],
    current: 0x110,
    dualMono: true,
    dualMonoSub: false,
  });

  test.each([
    { name: '1 つ目の音声の ES', choices: twoStreams, value: 'stream:272', slot: 1 },
    { name: '2 つ目の音声の ES', choices: twoStreams, value: 'stream:273', slot: 2 },
    { name: 'デュアルモノの主音声', choices: dualMono, value: 'dualMono:main', slot: 1 },
    { name: 'デュアルモノの副音声', choices: dualMono, value: 'dualMono:sub', slot: 2 },
  ])('$name は、kyusu の第 $slot 音声に当たる', ({ choices, value, slot }) => {
    const choice = choices.find((candidate) => candidate.value === value);
    expect(choice && audioSlot(choice)).toBe(slot);
    expect(choiceForSlot(choices, slot as 1 | 2)).toBe(value);
  });

  test('選べる音声がなければ、kyusu の音声の番号に当たるものはない', () => {
    expect(choiceForSlot([], 2)).toBeNull();
  });
});

describe('kyusuAudioChoices', () => {
  const stereo = (languages: string[]) => ({ componentType: 3, languages });
  test.each([
    {
      name: '構成が分からない',
      audios: null,
      want: [
        { slot: 1, kind: 'unknown', language: null },
        { slot: 2, kind: 'unknown', language: null },
      ],
    },
    {
      name: '構成を残す前の録画',
      audios: [],
      want: [
        { slot: 1, kind: 'unknown', language: null },
        { slot: 2, kind: 'unknown', language: null },
      ],
    },
    { name: '音声が 1 つ', audios: [stereo(['jpn'])], want: [] },
    {
      name: '二か国語放送 (音声の ES が 2 つ)',
      audios: [stereo(['jpn']), stereo(['eng'])],
      want: [
        { slot: 1, kind: 'stream', language: 'jpn' },
        { slot: 2, kind: 'stream', language: 'eng' },
      ],
    },
    {
      name: '2 つ目の音声に言語がない',
      audios: [stereo(['jpn']), stereo([])],
      want: [
        { slot: 1, kind: 'stream', language: 'jpn' },
        { slot: 2, kind: 'stream', language: null },
      ],
    },
    {
      name: 'デュアルモノ',
      audios: [{ componentType: 2, languages: ['jpn', 'eng'] }],
      want: [
        { slot: 1, kind: 'dualMono', language: 'jpn' },
        { slot: 2, kind: 'dualMono', language: 'eng' },
      ],
    },
    {
      name: '言語が 1 つだけのデュアルモノ',
      audios: [{ componentType: 2, languages: ['jpn'] }],
      want: [
        { slot: 1, kind: 'dualMono', language: 'jpn' },
        { slot: 2, kind: 'dualMono', language: null },
      ],
    },
  ])('$name', ({ audios, want }) => {
    expect(kyusuAudioChoices(audios)).toEqual(want);
  });
});

describe('choiceForComponent', () => {
  test.each([
    {
      name: 'ES が 2 つあれば、component_tag の ES を選ぶ',
      tracks: {
        available: [mono, commentary],
        current: 0x110,
        dualMono: false,
        dualMonoSub: false,
      },
      componentTag: 0x11,
      channelId: undefined,
      want: 'stream:273',
    },
    {
      name: '聞いている ES のデュアルモノでは、channel_id 2 で副音声を選ぶ',
      tracks: { available: [bilingual], current: 0x110, dualMono: true, dualMonoSub: false },
      componentTag: 0x10,
      channelId: 2,
      want: 'dualMono:sub',
    },
    {
      name: '聞いている ES のデュアルモノでは、channel_id 1 で主音声を選ぶ',
      tracks: { available: [bilingual], current: 0x110, dualMono: true, dualMonoSub: true },
      componentTag: 0x10,
      channelId: 1,
      want: 'dualMono:main',
    },
    {
      name: '聞いている ES のデュアルモノでは、channel_id がなければ主音声に戻す',
      tracks: { available: [bilingual], current: 0x110, dualMono: true, dualMonoSub: true },
      componentTag: 0x10,
      channelId: undefined,
      want: 'dualMono:main',
    },
    {
      name: 'ない component_tag は選べない',
      tracks: { available: [mono], current: 0x110, dualMono: false, dualMonoSub: false },
      componentTag: 0x12,
      channelId: undefined,
      want: null,
    },
    {
      name: '音声を読む前は選べない',
      tracks: null,
      componentTag: 0x10,
      channelId: undefined,
      want: null,
    },
  ])('$name', ({ tracks, componentTag, channelId, want }) => {
    expect(choiceForComponent(tracks as AudioTracks | null, componentTag, channelId)).toBe(want);
  });
});

describe('decodedAudioTracks', () => {
  test.each([
    {
      name: '音声の ES が 2 つ',
      streams: [
        { pid: 0x110, componentTag: 0x10, dualMono: false },
        { pid: 0x111, componentTag: 0x11, dualMono: false },
      ],
      current: 0x111,
      channel: 'main' as const,
      want: ['stream:272', 'stream:273'],
      selected: 'stream:273',
    },
    {
      name: 'デュアルモノは、主と副の両方も選べる',
      streams: [{ pid: 0x110, componentTag: 0x10, dualMono: true }],
      current: 0x110,
      channel: 'both' as const,
      want: ['dualMono:main', 'dualMono:sub', 'dualMono:both'],
      selected: 'dualMono:both',
    },
    {
      name: 'デュアルモノの副音声',
      streams: [{ pid: 0x110, componentTag: 0x10, dualMono: true }],
      current: 0x110,
      channel: 'sub' as const,
      want: ['dualMono:main', 'dualMono:sub', 'dualMono:both'],
      selected: 'dualMono:sub',
    },
    {
      name: '選べる音声が 1 つ',
      streams: [{ pid: 0x110, componentTag: 0x10, dualMono: false }],
      current: 0x110,
      channel: 'both' as const,
      want: [],
      selected: 'stream:272',
    },
  ])('$name', ({ streams, current, channel, want, selected }) => {
    const tracks = decodedAudioTracks(streams, current, channel);

    expect(audioChoices(tracks).map((choice) => choice.value)).toEqual(want);
    expect(currentAudioChoice(tracks)).toBe(selected);
  });

  test('主と副の両方は、ほかの経路へ移ったら主音声にする', () => {
    const choices = audioChoices(
      decodedAudioTracks([{ pid: 0x110, componentTag: 0x10, dualMono: true }], 0x110, 'both'),
    );
    const both = choices.find((choice) => choice.value === 'dualMono:both')!;

    expect(audioSlot(both)).toBe(1);
    expect(choiceForSlot(choices, 1)).toBe('dualMono:main');
  });

  test('データ放送の文書が選んだ音声は、デコードする経路でも選べる', () => {
    const tracks = decodedAudioTracks(
      [
        { pid: 0x110, componentTag: 0x10, dualMono: false },
        { pid: 0x111, componentTag: 0x11, dualMono: false },
      ],
      0x110,
      'main',
    );

    expect(choiceForComponent(tracks, 0x11)).toBe('stream:273');
  });
});
