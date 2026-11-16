import { MantineProvider } from '@mantine/core';
import { act, cleanup, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import '../i18n';
import {
  AutoCaptionOverlay,
  autoCaptionAt,
  autoCaptionPositions,
  CaptionTrackSelect,
  type AutoCaption,
} from './AutoCaptionOverlay';

afterEach(cleanup);

describe('autoCaptionPositions', () => {
  test('録画の時間軸の日時を、途切れた分を詰めた再生の位置にする', () => {
    const recordedFrom = '2026-11-16T10:00:00Z';
    // 10:01:00 から 10:01:30 まで途切れた。
    const gaps = [{ from: '2026-11-16T10:01:00Z', to: '2026-11-16T10:01:30Z' }];

    expect(
      autoCaptionPositions(recordedFrom, gaps, [
        { from: '2026-11-16T10:00:10Z', to: '2026-11-16T10:00:12.5Z', text: 'あ' },
        { from: '2026-11-16T10:02:00Z', to: '2026-11-16T10:02:03Z', text: 'い' },
      ]),
    ).toEqual([
      { from: 10, to: 12.5, text: 'あ' },
      { from: 90, to: 93, text: 'い' },
    ]);
  });
});

describe('autoCaptionAt', () => {
  const captions: AutoCaption[] = [
    { from: 1, to: 3, text: 'あ' },
    { from: 2.5, to: 4, text: 'い' },
    { from: 10, to: 12, text: 'う' },
  ];

  test.each<{ name: string; position: number; want: string | null }>([
    { name: '最初の字幕より前', position: 0.5, want: null },
    { name: '字幕の始まり', position: 1, want: 'あ' },
    { name: '重なっていれば、後に始まったものを出す', position: 2.8, want: 'い' },
    { name: '後の字幕が終わっても、前の字幕が続いていれば出さない', position: 3.5, want: 'い' },
    { name: '字幕の終わりは含めない', position: 4, want: null },
    { name: '字幕の間', position: 7, want: null },
    { name: '最後の字幕', position: 11, want: 'う' },
    { name: '最後の字幕の後', position: 100, want: null },
  ])('$name', ({ position, want }) => {
    expect(autoCaptionAt(captions, position)?.text ?? null).toBe(want);
  });

  test('字幕がなければ null', () => {
    expect(autoCaptionAt([], 1)).toBeNull();
  });
});

describe('AutoCaptionOverlay', () => {
  let frames: FrameRequestCallback[] = [];
  beforeEach(() => {
    frames = [];
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
      frames.push(callback);

      return frames.length;
    });
    vi.stubGlobal('cancelAnimationFrame', () => {});

    return () => vi.unstubAllGlobals();
  });
  const step = () => {
    const pending = frames;
    frames = [];
    act(() => {
      for (const callback of pending) {
        callback(0);
      }
    });
  };

  test('再生の位置に合う字幕を、「自動生成」の印と一緒に出し、位置が外れたら消す', () => {
    let position = 10.5;
    render(
      <MantineProvider env="test">
        <AutoCaptionOverlay
          recordedFrom="2026-11-16T10:00:00Z"
          gaps={[]}
          captions={[
            { from: '2026-11-16T10:00:10Z', to: '2026-11-16T10:00:12Z', text: 'こんにちは' },
          ]}
          clock={() => position}
        />
      </MantineProvider>,
    );
    step();
    expect(screen.getByText('こんにちは')).toBeTruthy();
    expect(screen.getByText('自動生成')).toBeTruthy();

    position = 13;
    step();
    expect(screen.queryByText('こんにちは')).toBeNull();
  });
});

describe('CaptionTrackSelect', () => {
  test('自動生成した字幕がなければ出さない', () => {
    render(
      <MantineProvider env="test">
        <CaptionTrackSelect available={false} value="broadcast" onChange={() => {}} />
      </MantineProvider>,
    );

    expect(screen.queryByText('字幕の種類')).toBeNull();
  });

  test('自動生成した字幕があれば、放送と自動生成から選べる', () => {
    render(
      <MantineProvider env="test">
        <CaptionTrackSelect available value="auto" onChange={() => {}} />
      </MantineProvider>,
    );

    expect(screen.getByText('字幕の種類')).toBeTruthy();
    expect(screen.getByDisplayValue('自動生成')).toBeTruthy();
  });
});
