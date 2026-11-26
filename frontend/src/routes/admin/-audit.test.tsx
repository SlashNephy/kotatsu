import { act, cleanup, fireEvent, screen, waitFor } from '@testing-library/react';
import type { Variables } from 'relay-runtime';
import { afterEach, describe, expect, test } from 'vitest';

import { createTestEnvironment, renderRoute } from '../../test/route';
import { Route } from './audit';

afterEach(() => {
  cleanup();
});

// serve は、記録のない監査ログを返し、読んだときの絞り込みを filters に残す環境を作る。
function serve() {
  const filters: Variables[] = [];
  const environment = createTestEnvironment((request, variables) => {
    if (request.name === 'auditQuery') {
      filters.push(variables.filter as Variables);

      return {
        data: {
          auditEvents: { edges: [], pageInfo: { hasNextPage: false, endCursor: null } },
          users: [],
        },
      };
    }

    return { data: {} };
  });

  return { environment, filters };
}

// actionInput は、操作の種類で絞る欄。
function actionInput(): HTMLInputElement {
  return screen.getByRole('textbox', { name: '操作の種類' });
}

describe('AuditPage (#2146)', () => {
  test('URL の絞り込みで読み、入力にも出す', async () => {
    const { environment, filters } = serve();
    await renderRoute(Route, '/admin/audit', '/admin/audit?action=rule.', environment);

    expect(actionInput().value).toBe('rule.');
    expect(filters.at(-1)).toEqual({ actionPrefix: 'rule.', actorUserId: null });
  });

  test('絞り込むボタンで、URL を置き換える', async () => {
    const { environment, filters } = serve();
    const { router } = await renderRoute(Route, '/admin/audit', '/admin/audit', environment);
    const historyLength = router.history.length;

    fireEvent.change(actionInput(), { target: { value: ' user. ' } });
    // 入力の途中では、絞り込みを変えない。
    expect(router.state.location.search).toEqual({});

    fireEvent.click(screen.getByRole('button', { name: '絞り込む' }));

    await waitFor(() => expect(router.state.location.search).toEqual({ action: 'user.' }));
    await waitFor(() =>
      expect(filters.at(-1)).toEqual({ actionPrefix: 'user.', actorUserId: null }),
    );
    expect(router.history.length).toBe(historyLength);
  });

  test('入力の外で URL の絞り込みが変わったら、入力を合わせる', async () => {
    const { environment } = serve();
    const { router } = await renderRoute(
      Route,
      '/admin/audit',
      '/admin/audit?action=rule.',
      environment,
    );

    await act(() => router.navigate({ to: '/admin/audit', search: {} }));

    expect(actionInput().value).toBe('');
  });
});
