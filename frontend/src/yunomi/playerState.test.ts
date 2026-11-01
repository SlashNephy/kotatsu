import { cleanup, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';

import {
  documentTitle,
  playerEvents,
  usePlayerEvent,
  usePlayerEvents,
  type PlayerEventMap,
  type PlayerSnapshot,
} from './playerState';

const state: PlayerSnapshot = {
  kind: 'LIVE',
  target: 'U2VydmljZTozMjczNjAxMDI0',
  serviceName: 'ＫＯＴＡＴＳＵテレビ',
  program: { id: 'UHJvZ3JhbTox', name: '🈑こたつのニュース' },
  artwork: null,
  paused: false,
  position: null,
  duration: null,
  playbackRate: 1,
  volume: 0.8,
  muted: false,
  audios: [{ id: '1', label: '主音声' }],
  audio: '1',
  dataBroadcast: false,
  keyGroups: [],
};

const nextProgram = { id: 'UHJvZ3JhbToy', name: 'こたつの天気' };

afterEach(cleanup);

describe('playerEvents', () => {
  test.each<{ name: string; current: PlayerSnapshot; want: ReturnType<typeof playerEvents> }>([
    {
      name: '再生位置と長さが変わっただけなら、何も出さない',
      current: { ...state, position: 12, duration: 1800 },
      want: [],
    },
    {
      name: '中身の同じ配列を作り直しただけなら、何も出さない',
      current: { ...state, audios: [{ id: '1', label: '主音声' }], keyGroups: [] },
      want: [],
    },
    {
      name: '番組が変わった',
      current: { ...state, program: nextProgram },
      want: [
        { type: 'programChanged', payload: { previous: state.program, current: nextProgram } },
      ],
    },
    {
      name: '番組表から番組がなくなった',
      current: { ...state, program: null },
      want: [{ type: 'programChanged', payload: { previous: state.program, current: null } }],
    },
    {
      name: '一時停止とミュートが同時に変わった',
      current: { ...state, paused: true, muted: true },
      want: [
        { type: 'pausedChanged', payload: { paused: true } },
        { type: 'volumeChanged', payload: { volume: 0.8, muted: true } },
      ],
    },
    {
      name: '選べる音声が増えた',
      current: {
        ...state,
        audios: [
          { id: '1', label: '主音声' },
          { id: '2', label: '副音声' },
        ],
      },
      want: [
        {
          type: 'audioChanged',
          payload: {
            audios: [
              { id: '1', label: '主音声' },
              { id: '2', label: '副音声' },
            ],
            audio: '1',
          },
        },
      ],
    },
    {
      name: 'データ放送を出し、文書がボタンの組を決めた',
      current: { ...state, dataBroadcast: true, keyGroups: ['basic', 'data-button'] },
      want: [
        {
          type: 'dataBroadcastChanged',
          payload: { shown: true, keyGroups: ['basic', 'data-button'] },
        },
      ],
    },
  ])('$name', ({ current, want }) => {
    expect(playerEvents(state, current)).toEqual(want);
  });
});

describe('usePlayerEvents', () => {
  test('状態が変わったときだけ、購読している種類のイベントを配る。購読をやめたら配らない', () => {
    const onProgram = vi.fn<(payload: PlayerEventMap['programChanged']) => void>();
    const onPlayback =
      vi.fn<(payload: PlayerEventMap['pausedChanged'] | PlayerEventMap['volumeChanged']) => void>();
    const { rerender } = renderHook(
      ({ current, subscribed }) => {
        const events = usePlayerEvents(current);
        usePlayerEvent(events, 'programChanged', subscribed ? onProgram : () => {});
        usePlayerEvent(events, ['pausedChanged', 'volumeChanged'], onPlayback);
      },
      { initialProps: { current: state, subscribed: true } },
    );
    // 描いただけでは、何も出さない。
    expect(onProgram).not.toHaveBeenCalled();
    expect(onPlayback).not.toHaveBeenCalled();

    // 同じ番組のまま描き直しても、出さない。
    rerender({ current: { ...state }, subscribed: true });
    expect(onProgram).not.toHaveBeenCalled();

    rerender({ current: { ...state, program: nextProgram }, subscribed: true });
    expect(onProgram).toHaveBeenCalledExactlyOnceWith({
      previous: state.program,
      current: nextProgram,
    });

    rerender({ current: { ...state, program: nextProgram, paused: true }, subscribed: true });
    rerender({
      current: { ...state, program: nextProgram, paused: true, volume: 0.5 },
      subscribed: true,
    });
    expect(onPlayback.mock.calls).toEqual([[{ paused: true }], [{ volume: 0.5, muted: false }]]);

    // 購読する関数を差し替えたら、前の関数には配らない。
    rerender({ current: state, subscribed: false });
    expect(onProgram).toHaveBeenCalledTimes(1);
  });
});

describe('documentTitle', () => {
  test.each<{ name: string; state: PlayerSnapshot; want: (string | null)[] }>([
    {
      name: 'ライブでは、番組名に局の名前を添える',
      state,
      want: ['こたつのニュース', 'KOTATSUテレビ'],
    },
    {
      name: 'ライブで番組が分からなければ、局の名前だけ',
      state: { ...state, program: null },
      want: [null, 'KOTATSUテレビ'],
    },
    {
      name: '録画では、番組名だけ',
      state: { ...state, kind: 'RECORDING' },
      want: ['こたつのニュース', null],
    },
  ])('$name', ({ state, want }) => {
    expect(documentTitle(state)).toEqual(want);
  });
});
