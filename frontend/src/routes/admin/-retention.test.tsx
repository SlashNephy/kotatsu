import { act, cleanup, fireEvent, screen } from '@testing-library/react';
import { commitLocalUpdate, type Environment } from 'relay-runtime';
import { afterEach, describe, expect, test, vi } from 'vitest';

import { createTestEnvironment, renderRoute } from '../../test/route';
import { Route } from './retention';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function settingsResponse() {
  return {
    data: {
      systemSettings: {
        archiveRetentionDays: 30,
        notificationRetentionDays: 90,
        watchProgressRetentionDays: 365,
        networkActivityRetentionDays: 7,
        updatedAt: '2026-11-11T00:00:00Z',
      },
    },
  };
}

// readAgain は、ほかの管理者が保存した値を読み直したことにして、ストアの過去の番組を残す日数を days にする。
function readAgain(environment: Environment, days: number) {
  act(() =>
    commitLocalUpdate(environment, (store) => {
      store.getRoot().getLinkedRecord('systemSettings')?.setValue(days, 'archiveRetentionDays');
    }),
  );
}

// field は、保持期間の入力のうち、画面の順 (過去の番組、通知、再生位置、ネットワークの記録) で index 番目を返す。
function field(index: number): HTMLElement {
  const input = screen.getAllByRole('textbox', { name: '保持期間（日）' })[index];
  if (input === undefined) {
    throw new Error(`retention field ${index} not found`);
  }

  return input;
}

describe('保持期間 (#1452)', () => {
  test('触っていない欄は、読み直した値に合わせる', async () => {
    const environment = createTestEnvironment(() => settingsResponse());
    await renderRoute(Route, '/admin/retention', '/admin/retention', environment);
    await screen.findAllByRole('textbox', { name: '保持期間（日）' });
    expect(field(0)).toHaveProperty('value', '30');

    readAgain(environment, 60);

    expect(field(0)).toHaveProperty('value', '60');
  });

  test('書きかけの欄は、読み直しても捨てない', async () => {
    const environment = createTestEnvironment(() => settingsResponse());
    await renderRoute(Route, '/admin/retention', '/admin/retention', environment);
    await screen.findAllByRole('textbox', { name: '保持期間（日）' });
    fireEvent.change(field(1), { target: { value: '120' } });

    readAgain(environment, 60);

    expect(field(1)).toHaveProperty('value', '120');
    expect(field(0)).toHaveProperty('value', '60');
  });

  test('欄を離れたら、その欄だけを保存し、知らせる (#1740)', async () => {
    const sent: unknown[] = [];
    const environment = createTestEnvironment((request, variables) => {
      if (request.name === 'retentionMutation') {
        sent.push(variables.input);

        return { data: { updateSystemSettings: { updatedAt: '2026-11-11T00:00:01Z' } } };
      }

      return settingsResponse();
    });
    await renderRoute(Route, '/admin/retention', '/admin/retention', environment);
    await screen.findAllByRole('textbox', { name: '保持期間（日）' });
    fireEvent.change(field(1), { target: { value: '120' } });
    expect(sent).toEqual([]);

    await act(async () => fireEvent.blur(field(1)));

    expect((await screen.findByRole('status')).textContent).toContain('保存しました');
    expect(sent).toEqual([{ notificationRetentionDays: 120 }]);
  });

  test('値を変えずに欄を離れても、保存も知らせもしない (#1740)', async () => {
    const sent: unknown[] = [];
    const environment = createTestEnvironment((request, variables) => {
      if (request.name === 'retentionMutation') {
        sent.push(variables.input);
      }

      return settingsResponse();
    });
    await renderRoute(Route, '/admin/retention', '/admin/retention', environment);
    await screen.findAllByRole('textbox', { name: '保持期間（日）' });

    await act(async () => fireEvent.blur(field(0)));

    expect(sent).toEqual([]);
    expect(screen.queryByRole('status')).toBeNull();
  });

  test('空にした欄は、理由を出して保存しない (#1740)', async () => {
    const sent: unknown[] = [];
    const environment = createTestEnvironment((request, variables) => {
      if (request.name === 'retentionMutation') {
        sent.push(variables.input);
      }

      return settingsResponse();
    });
    await renderRoute(Route, '/admin/retention', '/admin/retention', environment);
    await screen.findAllByRole('textbox', { name: '保持期間（日）' });
    fireEvent.change(field(3), { target: { value: '' } });

    await act(async () => fireEvent.keyDown(field(3), { key: 'Enter' }));

    expect(screen.getByText('入れてください')).toBeDefined();
    expect(sent).toEqual([]);
  });
});
