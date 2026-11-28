import { act, cleanup, fireEvent, screen } from '@testing-library/react';
import { commitLocalUpdate, type Environment, type GraphQLResponse } from 'relay-runtime';
import { afterEach, describe, expect, test, vi } from 'vitest';

import { createTestEnvironment, renderRoute, type Respond } from '../../test/route';
import { Route } from './recording';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function settingsResponse() {
  return {
    data: {
      systemSettings: {
        recordingDefaults: {
          poolId: null,
          fileNameTemplate: '{{.Name}}',
          startMarginSeconds: 0,
          endMarginSeconds: 0,
          priority: 0,
          joinSegments: true,
          detectCm: false,
          generateCaptions: false,
          transcodePreset: null,
          transcodeFrameRate: 'auto',
        },
        ruleDefaults: {
          avoidDuplicates: false,
          duplicateWindowDays: null,
          followEventRelay: true,
          bundleSameEpisodes: true,
        },
        historyMaxDrops: 0,
        tvtplayChaptersEnabled: false,
        cmDetector: 'FFMPEG',
        thumbnailPreviewsEnabled: true,
        captionThreads: 0,
        sleepControlEnabled: true,
        updatedAt: '2026-11-11T00:00:00Z',
      },
      systemStatus: { pools: [] },
      transcodePresets: [],
      // 同じ画面の、字幕の自動生成のモデルの欄 (#1682) も、同じ応答で読む。
      captionModels: [],
    },
  };
}

const saved = { data: { updateSystemSettings: { updatedAt: '2026-11-11T00:00:01Z' } } };

// respondWith は、録画の既定値を保存する mutation を onSave に渡し、ほかの問い合わせには、既定値か、ファイル名のプレビューを返す。
function respondWith(onSave: (input: unknown) => ReturnType<Respond> = () => saved) {
  return createTestEnvironment((request, variables) => {
    if (request.name === 'recordingMutation') {
      return onSave(variables.input);
    }
    if (request.name === 'FileNameTemplateFieldPreviewQuery') {
      return { data: { previewFileName: { fileName: 'x.m2ts', error: null } } };
    }

    return settingsResponse();
  });
}

// readAgain は、ほかの管理者が保存した値を読み直したことにして、ストアの録画の既定値の優先度を priority にする。
function readAgain(environment: Environment, priority: number) {
  act(() =>
    commitLocalUpdate(environment, (store) => {
      store
        .getRoot()
        .getLinkedRecord('systemSettings')
        ?.getLinkedRecord('recordingDefaults')
        ?.setValue(priority, 'priority');
    }),
  );
}

describe('録画の既定値 (#1420)', () => {
  test('触っていない欄は、読み直した値に合わせる', async () => {
    const environment = respondWith();
    await renderRoute(Route, '/admin/recording', '/admin/recording', environment);
    const priority = await screen.findByRole('textbox', { name: '優先度' });
    expect(priority).toHaveProperty('value', '0');

    readAgain(environment, 5);

    expect(screen.getByRole('textbox', { name: '優先度' })).toHaveProperty('value', '5');
  });

  test('書きかけの欄は、ほかの欄を読み直しても捨てない', async () => {
    const environment = respondWith();
    await renderRoute(Route, '/admin/recording', '/admin/recording', environment);
    const template = await screen.findByRole('textbox', { name: 'ファイル名のテンプレート' });
    fireEvent.change(template, { target: { value: '{{.Name}}_{{.Service}}' } });

    readAgain(environment, 5);

    expect(screen.getByRole('textbox', { name: 'ファイル名のテンプレート' })).toHaveProperty(
      'value',
      '{{.Name}}_{{.Service}}',
    );
    expect(screen.getByRole('textbox', { name: '優先度' })).toHaveProperty('value', '5');
  });
});

describe('数と文字の欄は、欄を離れたときか Enter で保存する (#1740)', () => {
  test('打っている途中は保存せず、欄を離れたら、その欄だけを送って通知する', async () => {
    const sent: unknown[] = [];
    const environment = respondWith((input) => {
      sent.push(input);

      return saved;
    });
    await renderRoute(Route, '/admin/recording', '/admin/recording', environment);
    const maxDrops = await screen.findByRole('textbox', {
      name: '録画済みとみなす映像・音声のドロップの上限（件）',
    });
    expect(maxDrops).toHaveProperty('value', '0');

    fireEvent.change(maxDrops, { target: { value: '4' } });
    expect(sent).toEqual([]);

    await act(async () => fireEvent.blur(maxDrops));

    expect((await screen.findByRole('status')).textContent).toContain('保存しました');
    expect(sent).toEqual([{ historyMaxDrops: 4 }]);
  });

  test('前後の空白を除いたテンプレートを、Enter で保存する', async () => {
    const sent: unknown[] = [];
    const environment = respondWith((input) => {
      sent.push(input);

      return saved;
    });
    await renderRoute(Route, '/admin/recording', '/admin/recording', environment);
    const template = await screen.findByRole('textbox', { name: 'ファイル名のテンプレート' });
    fireEvent.change(template, { target: { value: '{{.Name}}_{{.Service}} ' } });

    await act(async () => fireEvent.keyDown(template, { key: 'Enter' }));
    await screen.findByRole('status');

    expect(sent).toEqual([{ recordingDefaults: { fileNameTemplate: '{{.Name}}_{{.Service}}' } }]);
    expect(template).toHaveProperty('value', '{{.Name}}_{{.Service}}');
  });

  test('範囲を外れた値は、欄の下に理由を出して保存しない', async () => {
    const sent: unknown[] = [];
    const environment = respondWith((input) => {
      sent.push(input);

      return saved;
    });
    await renderRoute(Route, '/admin/recording', '/admin/recording', environment);
    const priority = await screen.findByRole('textbox', { name: '優先度' });
    fireEvent.change(priority, { target: { value: '11' } });

    await act(async () => fireEvent.blur(priority));

    expect(screen.getByText('-10〜10 の整数で入れてください')).toBeDefined();
    expect(priority).toHaveProperty('value', '11');
    expect(sent).toEqual([]);
  });

  test('空のテンプレートは、理由を出して保存しない', async () => {
    const sent: unknown[] = [];
    const environment = respondWith((input) => {
      sent.push(input);

      return saved;
    });
    await renderRoute(Route, '/admin/recording', '/admin/recording', environment);
    const template = await screen.findByRole('textbox', { name: 'ファイル名のテンプレート' });
    fireEvent.change(template, { target: { value: '  ' } });

    await act(async () => fireEvent.blur(template));

    expect(screen.getByText('入れてください')).toBeDefined();
    expect(sent).toEqual([]);
  });

  test('Enter で保存した後に欄を離れても、同じ値を重ねて送らない', async () => {
    const sent: unknown[] = [];
    // 保存には応えず、保存の途中のままにする。
    const environment = respondWith((input) => {
      sent.push(input);

      return undefined;
    });
    await renderRoute(Route, '/admin/recording', '/admin/recording', environment);
    const priority = await screen.findByRole('textbox', { name: '優先度' });
    fireEvent.change(priority, { target: { value: '3' } });

    await act(async () => fireEvent.keyDown(priority, { key: 'Enter' }));
    await act(async () => fireEvent.blur(priority));

    expect(sent).toEqual([{ recordingDefaults: { priority: 3 } }]);
  });

  test('保存に失敗したら、打った値を残して理由を伝え、もう一度欄を離れれば保存し直す', async () => {
    let fail = true;
    const sent: unknown[] = [];
    const environment = respondWith((input) => {
      sent.push(input);

      return fail ? new Error('unavailable') : saved;
    });
    await renderRoute(Route, '/admin/recording', '/admin/recording', environment);
    const priority = await screen.findByRole('textbox', { name: '優先度' });
    fireEvent.change(priority, { target: { value: '3' } });

    await act(async () => fireEvent.blur(priority));

    expect((await screen.findByRole('alert')).textContent).toContain('保存できませんでした');
    expect(priority).toHaveProperty('value', '3');

    fail = false;
    await act(async () => fireEvent.blur(priority));

    expect((await screen.findByRole('status')).textContent).toContain('保存しました');
    expect(sent).toEqual([
      { recordingDefaults: { priority: 3 } },
      { recordingDefaults: { priority: 3 } },
    ]);
  });
});

describe('字幕の自動生成のスレッドの数 (#1869)', () => {
  test('入れた数を、欄を離れたら送る', async () => {
    const sent: unknown[] = [];
    const environment = respondWith((input) => {
      sent.push(input);

      return saved;
    });
    await renderRoute(Route, '/admin/recording', '/admin/recording', environment);
    const threads = await screen.findByRole('textbox', {
      name: '字幕の自動生成に使うスレッドの数',
    });
    expect(threads).toHaveProperty('value', '0');

    fireEvent.change(threads, { target: { value: '4' } });
    await act(async () => fireEvent.blur(threads));
    await screen.findByRole('status');

    expect(sent).toEqual([{ captionThreads: 4 }]);
  });
});

describe('切り替えと選ぶ項目は、変えたらすぐ保存する (#1740)', () => {
  test('同じ回をまとめる設定を切ったら、その項目だけをすぐ保存する (#1498)', async () => {
    const sent: unknown[] = [];
    const environment = respondWith((input) => {
      sent.push(input);

      return saved;
    });
    await renderRoute(Route, '/admin/recording', '/admin/recording', environment);
    const bundle = await screen.findByRole('switch', {
      name: '局をまたいで同じ回をまとめ、最初の放送だけを予約する',
    });
    expect(bundle).toHaveProperty('checked', true);

    await act(async () => fireEvent.click(bundle));
    await screen.findByRole('status');

    expect(sent).toEqual([{ ruleDefaults: { bundleSameEpisodes: false } }]);
  });

  test('選んだフレームレートをすぐ保存する (#1509)', async () => {
    const sent: unknown[] = [];
    const environment = respondWith((input) => {
      sent.push(input);

      return saved;
    });
    await renderRoute(Route, '/admin/recording', '/admin/recording', environment);
    const frameRate = await screen.findByRole('combobox', { name: '変換のフレームレート' });
    expect(frameRate).toHaveProperty('value', '自動（映像から30か60を選ぶ）');

    fireEvent.click(frameRate);
    await act(async () => fireEvent.click(await screen.findByRole('option', { name: '60fps' })));
    await screen.findByRole('status');

    expect(sent).toEqual([{ recordingDefaults: { transcodeFrameRate: '60' } }]);
  });

  test('書きかけの欄があっても、切り替えた項目だけを送り、書きかけは残す', async () => {
    const sent: unknown[] = [];
    const environment = respondWith((input) => {
      sent.push(input);

      return saved;
    });
    await renderRoute(Route, '/admin/recording', '/admin/recording', environment);
    // 欄を離れずに、切り替えを押したことにする (fireEvent は、押したときに欄を離れさせない)。
    fireEvent.change(await screen.findByRole('textbox', { name: '優先度' }), {
      target: { value: '3' },
    });

    await act(async () =>
      fireEvent.click(screen.getByRole('switch', { name: /^録画が終わったら字幕を自動生成する/ })),
    );
    await screen.findByRole('status');

    expect(sent).toEqual([{ recordingDefaults: { generateCaptions: true } }]);
    expect(screen.getByRole('textbox', { name: '優先度' })).toHaveProperty('value', '3');
  });

  test('続けて変えたら、前の保存を待って順に送り、通知は 1 つに差し替える', async () => {
    const sent: unknown[] = [];
    let release: () => void = () => {};
    const environment = respondWith((input) => {
      sent.push(input);
      if (sent.length === 1) {
        return new Promise<GraphQLResponse>((resolve) => {
          release = () => resolve(saved);
        });
      }

      return saved;
    });
    await renderRoute(Route, '/admin/recording', '/admin/recording', environment);
    const detectCm = await screen.findByRole('switch', {
      name: /^録画が終わったらCMの区間を解析する/,
    });
    const joinSegments = screen.getByRole('switch', { name: /^分かれた録画のファイルを/ });

    await act(async () => fireEvent.click(detectCm));
    await act(async () => fireEvent.click(joinSegments));
    // 1 つ目の保存が終わるまで、2 つ目は送らない。
    expect(sent).toEqual([{ recordingDefaults: { detectCm: true } }]);

    await act(async () => release());
    await screen.findByRole('status');

    expect(sent).toEqual([
      { recordingDefaults: { detectCm: true } },
      { recordingDefaults: { joinSegments: false } },
    ]);
    expect(screen.getAllByRole('status')).toHaveLength(1);
  });

  // 設定ファイルから画面に移した切り替え (#1869) も、変えたらすぐ、その項目だけを送る。
  test.each([
    {
      name: 'プレビュー画像',
      label: /^シークバーのプレビュー画像を作る/,
      want: { thumbnailPreviewsEnabled: false },
    },
    {
      name: 'OS のスリープ',
      label: /^録画の間はスリープさせず/,
      want: { sleepControlEnabled: false },
    },
  ])('$name を切ったら、その項目だけを送る (#1869)', async ({ label, want }) => {
    const sent: unknown[] = [];
    const environment = respondWith((input) => {
      sent.push(input);

      return saved;
    });
    await renderRoute(Route, '/admin/recording', '/admin/recording', environment);
    const toggle = await screen.findByRole('switch', { name: label });
    expect(toggle).toHaveProperty('checked', true);

    await act(async () => fireEvent.click(toggle));
    await screen.findByRole('status');

    expect(sent).toEqual([want]);
  });

  test('保存に失敗したら、理由を伝え、変える前に戻す', async () => {
    const environment = respondWith(() => new Error('unavailable'));
    await renderRoute(Route, '/admin/recording', '/admin/recording', environment);
    const detectCm = await screen.findByRole('switch', {
      name: /^録画が終わったらCMの区間を解析する/,
    });

    await act(async () => fireEvent.click(detectCm));

    expect((await screen.findByRole('alert')).textContent).toContain('保存できませんでした');
    expect(detectCm).toHaveProperty('checked', false);
  });
});
