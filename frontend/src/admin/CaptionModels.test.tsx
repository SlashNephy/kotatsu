import { MantineProvider } from '@mantine/core';
import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { Suspense } from 'react';
import { RelayEnvironmentProvider } from 'react-relay';
import type { Variables } from 'relay-runtime';
import { afterEach, describe, expect, test } from 'vitest';

import { createTestEnvironment } from '../test/route';
import { CaptionModels } from './CaptionModels';

afterEach(cleanup);

const kotoba = {
  id: 'kotoba-whisper-v2.0-q5_0',
  name: 'kotoba-whisper v2.0 (q5_0)',
  size: 537819875,
  license: 'Apache-2.0',
  recommended: true,
};
const small = {
  id: 'whisper-small-q5_1',
  name: 'Whisper small (q5_1)',
  size: 190085487,
  license: 'MIT',
  recommended: false,
};
const silero = {
  id: 'silero-v5.1.2',
  name: 'Silero VAD v5.1.2',
  size: 885098,
  license: 'MIT',
  recommended: true,
};

type Slot = {
  kind: 'WHISPER' | 'VAD';
  state: string;
  overridden?: boolean;
  installed?: object | null;
  download?: object | null;
};

function slots(whisper: Slot, vad?: Slot) {
  return [
    { overridden: false, installed: null, download: null, models: [kotoba, small], ...whisper },
    {
      kind: 'VAD',
      state: 'NOT_PLACED',
      overridden: false,
      installed: null,
      download: null,
      models: [silero],
      ...vad,
    },
  ];
}

// renderModels は、captionModels に response を返す環境で、モデルの欄を出す。送った mutation を sent に残す。
async function renderModels(response: ReturnType<typeof slots>) {
  const sent: { name: string; variables: Variables }[] = [];
  const environment = createTestEnvironment((request, variables) => {
    if (request.operationKind === 'mutation') {
      sent.push({ name: request.name, variables });
      const field =
        request.name === 'CaptionModelsDownloadMutation'
          ? 'downloadCaptionModel'
          : request.name === 'CaptionModelsCancelMutation'
            ? 'cancelCaptionModelDownload'
            : 'deleteCaptionModel';

      return { data: { [field]: { kind: 'WHISPER', state: 'DOWNLOADING' } } };
    }

    return { data: { captionModels: response } };
  });
  await act(async () => {
    render(
      <MantineProvider env="test">
        <RelayEnvironmentProvider environment={environment}>
          <Suspense fallback={null}>
            <CaptionModels />
          </Suspense>
        </RelayEnvironmentProvider>
      </MantineProvider>,
    );
  });

  return sent;
}

function whisperSection() {
  const section = document.querySelector<HTMLElement>('[data-caption-model="WHISPER"]');
  if (section === null) {
    throw new Error('the whisper section is not rendered');
  }

  return within(section);
}

describe('CaptionModels (#1682)', () => {
  test.each<{ name: string; whisper: Slot; want: string[]; notWant?: string[] }>([
    {
      name: '置いていなければ、勧めるモデルを選んでダウンロードを出す',
      whisper: { kind: 'WHISPER', state: 'NOT_PLACED' },
      want: ['置いていない', 'ダウンロードする'],
      notWant: ['消す', '取りやめる'],
    },
    {
      name: 'ダウンロードしている間は、進み具合と取りやめを出す',
      whisper: {
        kind: 'WHISPER',
        state: 'DOWNLOADING',
        download: {
          model: { id: kotoba.id, name: kotoba.name },
          running: true,
          failed: false,
          progress: { ratio: 0.42, remainingSeconds: 30 },
        },
      },
      want: [
        'ダウンロード中',
        'kotoba-whisper v2.0 (q5_0)をダウンロードしています',
        '42%',
        '取りやめる',
      ],
      notWant: ['ダウンロードする', '消す'],
    },
    {
      name: '画面から置いたモデルは、名前と大きさを出し、置き換えと消すを出す',
      whisper: {
        kind: 'WHISPER',
        state: 'PLACED',
        installed: {
          model: { id: small.id, name: small.name },
          size: small.size,
          installedAt: '2026-11-16T03:00:00Z',
        },
      },
      want: [
        '置いた',
        'Whisper small (q5_1)（190.1 MB）を置いています',
        'ダウンロードして置き換える',
        '消す',
      ],
    },
    {
      name: '手で置いたファイルは、そのことを出す',
      whisper: {
        kind: 'WHISPER',
        state: 'PLACED',
        installed: { model: null, size: 1000000, installedAt: null },
      },
      want: ['手で置いたファイル（1.0 MB）を置いています'],
    },
    {
      name: '設定で別のファイルを使っていれば、そのことを出す',
      whisper: { kind: 'WHISPER', state: 'NOT_PLACED', overridden: true },
      want: ['設定（captions.whisper_model）で別のファイルを使っています'],
    },
    {
      name: '失敗したら、ジョブの画面で確かめられると出す',
      whisper: {
        kind: 'WHISPER',
        state: 'FAILED',
        download: {
          model: { id: kotoba.id, name: kotoba.name },
          running: false,
          failed: true,
          progress: null,
        },
      },
      want: ['失敗', 'kotoba-whisper v2.0 (q5_0)をダウンロードできませんでした'],
    },
  ])('$name', async ({ whisper, want, notWant }) => {
    await renderModels(slots(whisper));

    const text = document.querySelector('[data-caption-model="WHISPER"]')?.textContent ?? '';
    for (const part of want) {
      expect(text).toContain(part);
    }
    for (const part of notWant ?? []) {
      expect(text).not.toContain(part);
    }
  });

  test('ダウンロードは、選んだモデルの ID だけを送る', async () => {
    const sent = await renderModels(slots({ kind: 'WHISPER', state: 'NOT_PLACED' }));

    await act(async () => {
      fireEvent.click(whisperSection().getByRole('button', { name: 'ダウンロードする' }));
    });

    expect(sent).toEqual([
      { name: 'CaptionModelsDownloadMutation', variables: { id: 'kotoba-whisper-v2.0-q5_0' } },
    ]);
  });

  test('消すのは、確かめてから送る', async () => {
    const sent = await renderModels(
      slots({
        kind: 'WHISPER',
        state: 'PLACED',
        installed: {
          model: { id: small.id, name: small.name },
          size: small.size,
          installedAt: null,
        },
      }),
    );

    fireEvent.click(whisperSection().getByRole('button', { name: '消す' }));
    expect(sent).toEqual([]);
    const dialog = await screen.findByRole('dialog');
    await act(async () => {
      fireEvent.click(within(dialog).getByRole('button', { name: '消す' }));
    });

    expect(sent).toEqual([{ name: 'CaptionModelsDeleteMutation', variables: { kind: 'WHISPER' } }]);
  });
});
