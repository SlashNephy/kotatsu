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
  test('触っていないフォームは、読み直した値に合わせる', async () => {
    const environment = createTestEnvironment(() => settingsResponse());
    await renderRoute(Route, '/admin/retention', '/admin/retention', environment);
    await screen.findAllByRole('textbox', { name: '保持期間（日）' });
    expect(field(0)).toHaveProperty('value', '30');

    readAgain(environment, 60);

    expect(field(0)).toHaveProperty('value', '60');
  });

  test('触ったフォームの入力は、読み直しても捨てない', async () => {
    const environment = createTestEnvironment(() => settingsResponse());
    await renderRoute(Route, '/admin/retention', '/admin/retention', environment);
    await screen.findAllByRole('textbox', { name: '保持期間（日）' });
    fireEvent.change(field(1), { target: { value: '120' } });

    readAgain(environment, 60);

    expect(field(1)).toHaveProperty('value', '120');
    expect(field(0)).toHaveProperty('value', '30');
  });

  test('読み直した後に保存すると、触っていない項目は読み直した値を送る', async () => {
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

    readAgain(environment, 60);
    await act(async () => fireEvent.click(screen.getByRole('button', { name: '保存' })));
    await screen.findByRole('status');

    expect(sent).toEqual([
      {
        archiveRetentionDays: 60,
        notificationRetentionDays: 90,
        watchProgressRetentionDays: 365,
        networkActivityRetentionDays: 7,
      },
    ]);
  });
});
