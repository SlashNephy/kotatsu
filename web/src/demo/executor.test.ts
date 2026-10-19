import type { ConcreteRequest } from 'relay-runtime';
import { describe, expect, it } from 'vitest';

import UserMenuQuery from '../auth/__generated__/UserMenuQuery.graphql';
import DataChangesSubscription from '../changes/__generated__/DataChangesSubscription.graphql';
import GuideViewEditorCreateMutation from '../guide/__generated__/GuideViewEditorCreateMutation.graphql';
import GuideViewEditorDeleteMutation from '../guide/__generated__/GuideViewEditorDeleteMutation.graphql';
import GuideViewEditorUpdateMutation from '../guide/__generated__/GuideViewEditorUpdateMutation.graphql';
import PinButtonMutation from '../guide/__generated__/PinButtonMutation.graphql';
import ProgramDetailQuery from '../guide/__generated__/ProgramDetailQuery.graphql';
import DashboardQuery from '../home/__generated__/DashboardQuery.graphql';
import RootViewerQuery from '../routes/__generated__/RootViewerQuery.graphql';
import searchProgramsQuery from '../routes/__generated__/searchProgramsQuery.graphql';
import searchViewerQuery from '../routes/__generated__/searchViewerQuery.graphql';
import channelServiceIdGuideQuery from '../routes/guide/__generated__/channelServiceIdGuideQuery.graphql';
import guideQuery from '../routes/guide/__generated__/guideQuery.graphql';
import liveServicesQuery from '../routes/live/__generated__/liveServicesQuery.graphql';
import WatchReserveButtonMutation from '../watch/__generated__/WatchReserveButtonMutation.graphql';
import { createRootValue } from './data';
import { createEventHub } from './events';
import { createExecutor, type Variables } from './executor';

const now = Date.parse('2026-10-19T12:00:00Z');

function newExecutor() {
  const events = createEventHub();

  return { events, executor: createExecutor(createRootValue(events, now)) };
}

function text(request: ConcreteRequest): string {
  return request.params.text ?? '';
}

const hour = 60 * 60 * 1000;
const at = (offset: number) => new Date(now + offset).toISOString();
// program は、デモの番組の 1 つ (こたつテレビの、3 日前の 4 時からの最初の番組)。
const program = 'Program:1024:1';

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
    {
      name: 'guide',
      request: guideQuery,
      variables: { from: at(0), to: at(24 * hour), view: null },
    },
    {
      name: 'channel guide',
      request: channelServiceIdGuideQuery,
      variables: { from: at(0), to: at(24 * hour), services: ['Service:1024'] },
    },
    { name: 'live list', request: liveServicesQuery, variables: { from: at(0), to: at(1000) } },
    { name: 'search viewer', request: searchViewerQuery, variables: {} },
    {
      name: 'search',
      request: searchProgramsQuery,
      variables: { conditions: {}, archive: null, first: 50, after: null },
    },
    { name: 'program detail', request: ProgramDetailQuery, variables: { id: program } },
    { name: 'pin', request: PinButtonMutation, variables: { id: 'Service:1040', pinned: true } },
    {
      name: 'watch reservation',
      request: WatchReserveButtonMutation,
      variables: { program, reserved: true },
    },
    {
      name: 'create guide view',
      request: GuideViewEditorCreateMutation,
      variables: { input: { name: 'アニメ', genres: [{ lv1: 7 }] } },
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

describe('demo guide and search', () => {
  it('fills the guide without gaps and keeps programs within the range', async () => {
    const { executor } = newExecutor();

    const result = await executor.execute(text(channelServiceIdGuideQuery), {
      from: at(0),
      to: at(24 * hour),
      services: ['Service:1024'],
    });

    const rows = (result.data as { guide: { programs: { startAt: string; endAt: string }[] }[] })
      .guide;
    expect(rows).toHaveLength(1);
    const programs = rows[0]!.programs;
    expect(Date.parse(programs[0]!.startAt)).toBeLessThanOrEqual(now);
    expect(Date.parse(programs.at(-1)!.endAt)).toBeGreaterThanOrEqual(now + 24 * hour);
    for (const [index, item] of programs.slice(1).entries()) {
      expect(item.startAt).toBe(programs[index]!.endAt);
    }
  });

  it('finds only programs that match the keyword', async () => {
    const { executor } = newExecutor();

    const result = await executor.execute(text(searchProgramsQuery), {
      conditions: { keywords: [{ text: 'こたつ探偵', fields: ['NAME'] }] },
      archive: null,
      first: 100,
      after: null,
    });

    const edges = (result.data as { searchPrograms: { edges: { node: { name: string } }[] } })
      .searchPrograms.edges;
    expect(edges.length).toBeGreaterThan(0);
    expect(edges.every(({ node }) => node.name.includes('こたつ探偵'))).toBe(true);
  });

  it('narrows the guide by the conditions of a guide view, and forgets it after deletion', async () => {
    const { executor } = newExecutor();
    const created = await executor.execute(text(GuideViewEditorCreateMutation), {
      input: { name: 'アニメ', genres: [{ lv1: 7 }], hideEmptyServices: true },
    });
    const id = (created.data as { createGuideView: { id: string } }).createGuideView.id;

    const viewed = await executor.execute(text(guideQuery), {
      from: at(0),
      to: at(24 * hour),
      view: id,
    });
    const rows = (viewed.data as { guide: { programs: { genres: { lv1: number }[] }[] }[] }).guide;
    expect(rows.length).toBeGreaterThan(0);
    expect(rows.every((row) => row.programs.every((item) => item.genres[0]!.lv1 === 7))).toBe(true);

    await executor.execute(text(GuideViewEditorUpdateMutation), {
      id,
      input: { name: 'ニュース', genres: [{ lv1: 0 }] },
    });
    await executor.execute(text(GuideViewEditorDeleteMutation), { id });
    const after = await executor.execute(text(guideQuery), {
      from: at(0),
      to: at(hour),
      view: null,
    });
    expect((after.data as { guideViews: unknown[] }).guideViews).toEqual([]);
  });

  it('keeps the pinned services in the order they were pinned', async () => {
    const { executor } = newExecutor();

    await executor.execute(text(PinButtonMutation), { id: 'Service:1024', pinned: false });
    await executor.execute(text(PinButtonMutation), { id: 'Service:1024', pinned: true });

    const result = await executor.execute(text(liveServicesQuery), { from: at(0), to: at(1000) });
    const pinned = (result.data as { pinnedServices: { id: string }[] }).pinnedServices;
    expect(pinned.map((item) => item.id)).toEqual(['Service:1032', 'Service:1024']);
  });
});
