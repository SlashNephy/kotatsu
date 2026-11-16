import { MantineProvider } from '@mantine/core';
import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import '../i18n';
import { AutoCaptionsStatus, autoCaptionsActive, type AutoCaptionsValue } from './AutoCaptions';

afterEach(cleanup);

function renderStatus(status: AutoCaptionsValue) {
  render(
    <MantineProvider env="test">
      <AutoCaptionsStatus status={status} />
    </MantineProvider>,
  );

  return screen.getByRole('status').textContent;
}

const generatedAt = '2026-11-16T10:30:00+09:00';

describe('AutoCaptionsStatus', () => {
  test.each<{ name: string; status: AutoCaptionsValue; want: string[]; notWant?: string[] }>([
    {
      name: '生成し終えたら、字幕の数とモデルを出し、再生画面で選べると添える',
      status: {
        state: 'GENERATED',
        generatedAt,
        model: 'ggml-model.bin',
        count: 120,
        progress: null,
      },
      want: [
        '120件の字幕を生成しました',
        '11/16 10:30にggml-model.binで生成しました',
        '再生画面の設定で「自動生成」を選ぶと出ます',
      ],
    },
    {
      name: '台詞がなければ、字幕がないと出す',
      status: {
        state: 'GENERATED',
        generatedAt,
        model: 'ggml-model.bin',
        count: 0,
        progress: null,
      },
      want: ['台詞が見つからず、字幕はありません'],
      notWant: ['再生画面の設定'],
    },
    {
      name: '生成している間は、進み具合を出し、前に生成した字幕を使っていると添える',
      status: {
        state: 'RUNNING',
        generatedAt,
        model: 'ggml-model.bin',
        count: 30,
        progress: { ratio: 0.5, remainingSeconds: 600 },
      },
      want: [
        '音声から字幕を生成しています',
        '50%',
        'いまは、11/16 10:30に生成した字幕（30件）を使っています',
      ],
    },
    {
      name: 'whisper.cpp とモデルが置かれていなければ、置き方を示す',
      status: { state: 'NOT_CONFIGURED', generatedAt: null, model: null, count: 0, progress: null },
      want: ['whisper.cppとモデルが、サーバーに置かれていません', 'docs/setup.md'],
    },
    {
      name: '失敗したら、ジョブの画面で確かめられると出す',
      status: { state: 'FAILED', generatedAt: null, model: null, count: 0, progress: null },
      want: ['生成できませんでした'],
    },
    {
      name: '生成していなければ、まだと出す',
      status: { state: 'NOT_GENERATED', generatedAt: null, model: null, count: 0, progress: null },
      want: ['まだ生成していません'],
    },
  ])('$name', ({ status, want, notWant }) => {
    const text = renderStatus(status);

    for (const part of want) {
      expect(text).toContain(part);
    }
    for (const part of notWant ?? []) {
      expect(text).not.toContain(part);
    }
  });
});

describe('autoCaptionsActive', () => {
  test.each<{ state: AutoCaptionsValue['state']; want: boolean }>([
    { state: 'QUEUED', want: true },
    { state: 'RUNNING', want: true },
    { state: 'GENERATED', want: false },
    { state: 'FAILED', want: false },
    { state: 'NOT_CONFIGURED', want: false },
    { state: 'NOT_GENERATED', want: false },
  ])('$state なら $want', ({ state, want }) => {
    expect(autoCaptionsActive(state)).toBe(want);
  });
});
