import { cleanup, screen } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import { createTestEnvironment, renderRoute } from '../test/route';
import { Route } from './network-activity';

afterEach(() => {
  cleanup();
});

describe('NetworkActivityPage', () => {
  test('通信の記録を読み込めなかったら、読み込めなかったと伝える', async () => {
    const environment = createTestEnvironment((request) =>
      request.name === 'networkActivityQuery' ? new Error('failed') : undefined,
    );
    await renderRoute(Route, '/network-activity', '/network-activity', environment);

    expect((await screen.findByRole('alert')).textContent).toBe(
      '読み込めませんでした。時間をおいてやり直してください',
    );
  });
});
