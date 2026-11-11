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
        },
        ruleDefaults: { avoidDuplicates: false, duplicateWindowDays: null, followEventRelay: true },
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
