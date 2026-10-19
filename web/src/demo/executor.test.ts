import type { ConcreteRequest } from 'relay-runtime';
import { describe, expect, it } from 'vitest';

import UserMenuQuery from '../auth/__generated__/UserMenuQuery.graphql';
import DataChangesSubscription from '../changes/__generated__/DataChangesSubscription.graphql';
import DashboardQuery from '../home/__generated__/DashboardQuery.graphql';
import RootViewerQuery from '../routes/__generated__/RootViewerQuery.graphql';
import { createRootValue } from './data';
import { createEventHub } from './events';
import { createExecutor, type Variables } from './executor';

function newExecutor() {
  const events = createEventHub();

  return { events, executor: createExecutor(createRootValue(events)) };
}

function text(request: ConcreteRequest): string {
  return request.params.text ?? '';
}

const now = Date.parse('2026-10-19T12:00:00Z');

// 画面が実際に送る query (Relay のコンパイラが生成した本文) を、デモの実行器に流す。
// スキーマや画面の query が変わって、デモのデータが答えられなくなったら落ちる。
describe('demo executor answers the queries of the screens', () => {
  const cases: { name: string; request: ConcreteRequest; variables: Variables }[] = [
    { name: 'header viewer', request: RootViewerQuery, variables: {} },
    { name: 'user menu', request: UserMenuQuery, variables: {} },
    {
      name: 'home',
      request: DashboardQuery,
      variables: {
        from: new Date(now).toISOString(),
        nowEnd: new Date(now + 1000).toISOString(),
        to: new Date(now + 7 * 24 * 60 * 60 * 1000).toISOString(),
        recordings: 12,
        upcoming: 5,
      },
    },
  ];

  it.each(cases)('$name', async ({ request, variables }) => {
    const { executor } = newExecutor();

    const result = await executor.execute(text(request), variables);

    expect(result.errors).toBeUndefined();
    expect(result.data).not.toBeNull();
  });
});

describe('demo viewerEvents', () => {
  it('delivers a published data change to the subscriber', async () => {
    const { events, executor } = newExecutor();
    const results = (await executor.subscribe(text(DataChangesSubscription), {}))[
      Symbol.asyncIterator
    ]();

    const next = results.next();
    // 購読を始めてから流す。listen はジェネレーターなので、最初の next で購読が始まる。
    await Promise.resolve();
    events.publish({
      dataChange: { channel: 'RESERVATIONS', version: 3 },
      viewerSettingsVersion: null,
      unreadNotificationCount: null,
    });

    const { value } = await next;
    expect(value).toEqual({
      data: {
        viewerEvents: {
          dataChange: { channel: 'RESERVATIONS', version: 3 },
          viewerSettingsVersion: null,
          unreadNotificationCount: null,
        },
      },
    });
    await results.return?.();
  });

  it('ends a pending wait when the subscriber stops', async () => {
    const { executor } = newExecutor();
    const results = (await executor.subscribe(text(DataChangesSubscription), {}))[
      Symbol.asyncIterator
    ]();

    const pending = results.next();
    await results.return?.();

    await expect(pending).resolves.toEqual({ done: true, value: undefined });
  });
});
