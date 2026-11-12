import { act, cleanup, fireEvent, screen } from '@testing-library/react';
import { commitLocalUpdate, type Environment } from 'relay-runtime';
import { afterEach, describe, expect, test, vi } from 'vitest';

import { createTestEnvironment, renderRoute } from '../../test/route';
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
          transcodePreset: null,
          transcodeFrameRate: 'auto',
        },
        ruleDefaults: {
          avoidDuplicates: false,
          duplicateWindowDays: null,
          followEventRelay: true,
          bundleSameEpisodes: true,
        },
        ruleUnmatchedGraceMinutes: 180,
        updatedAt: '2026-11-11T00:00:00Z',
      },
      systemStatus: { pools: [] },
      transcodePresets: [],
    },
  };
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
  test('触っていないフォームは、読み直した値に合わせる', async () => {
    const environment = createTestEnvironment(() => settingsResponse());
    await renderRoute(Route, '/admin/recording', '/admin/recording', environment);
    const priority = await screen.findByRole('textbox', { name: '優先度' });
    expect(priority).toHaveProperty('value', '0');

    readAgain(environment, 5);

    expect(screen.getByRole('textbox', { name: '優先度' })).toHaveProperty('value', '5');
  });

  test('触ったフォームの入力は、読み直しても捨てない', async () => {
    const environment = createTestEnvironment(() => settingsResponse());
    await renderRoute(Route, '/admin/recording', '/admin/recording', environment);
    const template = await screen.findByRole('textbox', { name: 'ファイル名のテンプレート' });
    fireEvent.change(template, { target: { value: '{{.Name}}_{{.Service}}' } });

    readAgain(environment, 5);

    expect(screen.getByRole('textbox', { name: 'ファイル名のテンプレート' })).toHaveProperty(
      'value',
      '{{.Name}}_{{.Service}}',
    );
    expect(screen.getByRole('textbox', { name: '優先度' })).toHaveProperty('value', '0');
  });

  test('前後に空白のあるテンプレートを保存しても、保存の後は読み直しに追従する', async () => {
    // template は、サーバーが持つファイル名のテンプレート。保存すると、送った値にする。
    let template = '{{.Name}}';
    const environment = createTestEnvironment((request, variables) => {
      if (request.name === 'recordingMutation') {
        template = (variables.input as { recordingDefaults: { fileNameTemplate: string } })
          .recordingDefaults.fileNameTemplate;

        return { data: { updateSystemSettings: { updatedAt: '2026-11-11T00:00:01Z' } } };
      }
      const response = settingsResponse();
      response.data.systemSettings.recordingDefaults.fileNameTemplate = template;

      return response;
    });
    await renderRoute(Route, '/admin/recording', '/admin/recording', environment);
    fireEvent.change(await screen.findByRole('textbox', { name: 'ファイル名のテンプレート' }), {
      target: { value: '{{.Name}}_{{.Service}} ' },
    });
    await act(async () => fireEvent.click(screen.getByRole('button', { name: '保存' })));
    await screen.findByRole('status');

    readAgain(environment, 5);

    expect(screen.getByRole('textbox', { name: '優先度' })).toHaveProperty('value', '5');
  });
});

describe('ルールから外れた予約を残す時間 (#1501)', () => {
  test('入れた分を保存し、0 も送る', async () => {
    let sent: unknown = null;
    const environment = createTestEnvironment((request, variables) => {
      if (request.name === 'recordingMutation') {
        sent = (variables.input as { ruleUnmatchedGraceMinutes?: number })
          .ruleUnmatchedGraceMinutes;

        return { data: { updateSystemSettings: { updatedAt: '2026-11-11T00:00:01Z' } } };
      }

      return settingsResponse();
    });
    await renderRoute(Route, '/admin/recording', '/admin/recording', environment);
    const grace = await screen.findByRole('textbox', {
      name: 'ルールから外れた予約を残す時間（放送前の分）',
    });
    expect(grace).toHaveProperty('value', '180');

    fireEvent.change(grace, { target: { value: '0' } });
    await act(async () => fireEvent.click(screen.getByRole('button', { name: '保存' })));
    await screen.findByRole('status');

    expect(sent).toBe(0);
  });
});

describe('ルールを作るときの既定値の同じ回の束ね (#1498)', () => {
  test('読み込んだ値から始め、切ったら切ったことを保存する', async () => {
    let sent: unknown = null;
    const environment = createTestEnvironment((request, variables) => {
      if (request.name === 'recordingMutation') {
        sent = (variables.input as { ruleDefaults: { bundleSameEpisodes?: boolean } }).ruleDefaults
          .bundleSameEpisodes;

        return { data: { updateSystemSettings: { updatedAt: '2026-11-11T00:00:01Z' } } };
      }

      return settingsResponse();
    });
    await renderRoute(Route, '/admin/recording', '/admin/recording', environment);
    const bundle = await screen.findByRole('switch', {
      name: '局をまたいで同じ回を束ね、最初の放送だけを予約する',
    });
    expect(bundle).toHaveProperty('checked', true);

    fireEvent.click(bundle);
    await act(async () => fireEvent.click(screen.getByRole('button', { name: '保存' })));
    await screen.findByRole('status');

    expect(sent).toBe(false);
  });
});

describe('変換のコマ数 (#1509)', () => {
  test('読んだ全体の既定値を出し、選んだコマ数を保存する', async () => {
    let sent: unknown = null;
    const environment = createTestEnvironment((request, variables) => {
      if (request.name === 'recordingMutation') {
        sent = (variables.input as { recordingDefaults: { transcodeFrameRate?: string } })
          .recordingDefaults.transcodeFrameRate;

        return { data: { updateSystemSettings: { updatedAt: '2026-11-11T00:00:01Z' } } };
      }

      return settingsResponse();
    });
    await renderRoute(Route, '/admin/recording', '/admin/recording', environment);
    const frameRate = await screen.findByRole('combobox', { name: '変換のコマ数' });
    expect(frameRate).toHaveProperty('value', '自動（映像から30か60を選ぶ）');

    fireEvent.click(frameRate);
    fireEvent.click(await screen.findByRole('option', { name: '60コマ/秒' }));
    await act(async () => fireEvent.click(screen.getByRole('button', { name: '保存' })));
    await screen.findByRole('status');

    expect(sent).toBe('60');
  });
});
