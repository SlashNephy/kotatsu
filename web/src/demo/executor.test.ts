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
import FileNameTemplateFieldDefaultQuery from '../reservations/__generated__/FileNameTemplateFieldDefaultQuery.graphql';
import FileNameTemplateFieldHelpQuery from '../reservations/__generated__/FileNameTemplateFieldHelpQuery.graphql';
import FileNameTemplateFieldPreviewQuery from '../reservations/__generated__/FileNameTemplateFieldPreviewQuery.graphql';
import MarginFieldsQuery from '../reservations/__generated__/MarginFieldsQuery.graphql';
import ReservationSettingsDialogCreateMutation from '../reservations/__generated__/ReservationSettingsDialogCreateMutation.graphql';
import ReservationSettingsDialogMutation from '../reservations/__generated__/ReservationSettingsDialogMutation.graphql';
import ReservationSettingsDialogQuery from '../reservations/__generated__/ReservationSettingsDialogQuery.graphql';
import SkippedDuplicateListQuery from '../reservations/__generated__/SkippedDuplicateListQuery.graphql';
import TimeReservationDialogCreateMutation from '../reservations/__generated__/TimeReservationDialogCreateMutation.graphql';
import TimeReservationDialogQuery from '../reservations/__generated__/TimeReservationDialogQuery.graphql';
import TimeReservationDialogUpdateMutation from '../reservations/__generated__/TimeReservationDialogUpdateMutation.graphql';
import reservationsDeleteManyMutation from '../routes/__generated__/reservationsDeleteManyMutation.graphql';
import reservationsDeleteMutation from '../routes/__generated__/reservationsDeleteMutation.graphql';
import reservationsEnableMutation from '../routes/__generated__/reservationsEnableMutation.graphql';
import reservationsQuery from '../routes/__generated__/reservationsQuery.graphql';
import reservationsSetManyEnabledMutation from '../routes/__generated__/reservationsSetManyEnabledMutation.graphql';
import RuleIdQuery from '../routes/rules/__generated__/RuleIdQuery.graphql';
import rulesDeleteManyMutation from '../routes/rules/__generated__/rulesDeleteManyMutation.graphql';
import rulesEnableMutation from '../routes/rules/__generated__/rulesEnableMutation.graphql';
import rulesListQuery from '../routes/rules/__generated__/rulesListQuery.graphql';
import rulesSetManyEnabledMutation from '../routes/rules/__generated__/rulesSetManyEnabledMutation.graphql';
import RuleEditorCreateMutation from '../rules/__generated__/RuleEditorCreateMutation.graphql';
import RuleEditorDeleteMutation from '../rules/__generated__/RuleEditorDeleteMutation.graphql';
import RuleEditorUpdateMutation from '../rules/__generated__/RuleEditorUpdateMutation.graphql';
import RulePreviewQuery from '../rules/__generated__/RulePreviewQuery.graphql';
import WatchReminderSettingsMutation from '../watch/__generated__/WatchReminderSettingsMutation.graphql';
import WatchReservationListQuery from '../watch/__generated__/WatchReservationListQuery.graphql';
import ProgramDetailCancelMutation from '../guide/__generated__/ProgramDetailCancelMutation.graphql';
import ProgramDetailReserveMutation from '../guide/__generated__/ProgramDetailReserveMutation.graphql';
import WatchReserveButtonMutation from '../watch/__generated__/WatchReserveButtonMutation.graphql';
import { createRootValue } from './data';
import { generatePrograms } from './epg';
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
const programs = generatePrograms(now);
// past は、放送が終わった番組。upcoming は、これから放送する番組。
const past = programs.find((item) => item.end <= now)!.id;
const upcoming = programs.find((item) => item.start > now)!.id;
// firstReservation は、最初の状態にあるルール予約の 1 つ。rule は、ルールの入力の例。
const firstReservation = 'Reservation:3';
const rule = { name: 'アニメ', conditions: { genres: [{ lv1: 7 }] } };

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
    { name: 'program detail', request: ProgramDetailQuery, variables: { id: past } },
    { name: 'pin', request: PinButtonMutation, variables: { id: 'Service:1040', pinned: true } },
    {
      name: 'watch reservation',
      request: WatchReserveButtonMutation,
      variables: { program: upcoming, reserved: true },
    },
    {
      name: 'reservations',
      request: reservationsQuery,
      variables: {
        from: at(-hour),
        to: at(8 * 24 * hour),
        states: null,
        rule: null,
        ruleId: '',
        withRule: false,
        first: 50,
        after: null,
      },
    },
    {
      name: 'reservations of a rule',
      request: reservationsQuery,
      variables: {
        from: at(-hour),
        to: at(8 * 24 * hour),
        states: ['OK'],
        rule: 'Rule:1',
        ruleId: 'Rule:1',
        withRule: true,
        first: 50,
        after: null,
      },
    },
    { name: 'rules', request: rulesListQuery, variables: {} },
    { name: 'rule', request: RuleIdQuery, variables: { id: 'Rule:1', skip: false } },
    { name: 'new rule', request: RuleIdQuery, variables: { id: '', skip: true } },
    { name: 'rule preview', request: RulePreviewQuery, variables: { input: rule } },
    { name: 'create rule', request: RuleEditorCreateMutation, variables: { input: rule } },
    {
      name: 'update rule',
      request: RuleEditorUpdateMutation,
      variables: { id: 'Rule:1', input: rule },
    },
    { name: 'delete rule', request: RuleEditorDeleteMutation, variables: { id: 'Rule:1' } },
    {
      name: 'enable rule',
      request: rulesEnableMutation,
      variables: { id: 'Rule:1', enabled: false },
    },
    {
      name: 'enable rules',
      request: rulesSetManyEnabledMutation,
      variables: { ids: ['Rule:1'], enabled: false },
    },
    { name: 'delete rules', request: rulesDeleteManyMutation, variables: { ids: ['Rule:1'] } },
    { name: 'reserve', request: ProgramDetailReserveMutation, variables: { program: upcoming } },
    {
      name: 'reserve with settings',
      request: ReservationSettingsDialogCreateMutation,
      variables: {
        program: upcoming,
        settings: { startMarginSeconds: 60 },
        followEventRelay: true,
      },
    },
    { name: 'cancel', request: ProgramDetailCancelMutation, variables: { id: firstReservation } },
    {
      name: 'delete reservation',
      request: reservationsDeleteMutation,
      variables: { id: firstReservation },
    },
    {
      name: 'delete reservations',
      request: reservationsDeleteManyMutation,
      variables: { ids: [firstReservation] },
    },
    {
      name: 'enable reservation',
      request: reservationsEnableMutation,
      variables: { id: firstReservation, enabled: false },
    },
    {
      name: 'enable reservations',
      request: reservationsSetManyEnabledMutation,
      variables: { ids: [firstReservation], enabled: false },
    },
    {
      name: 'reservation settings',
      request: ReservationSettingsDialogQuery,
      variables: { id: firstReservation },
    },
    {
      name: 'update reservation settings',
      request: ReservationSettingsDialogMutation,
      variables: { id: firstReservation, settings: { priority: 1 }, followEventRelay: null },
    },
    { name: 'time reservation dialog', request: TimeReservationDialogQuery, variables: {} },
    {
      name: 'create time reservation',
      request: TimeReservationDialogCreateMutation,
      variables: {
        input: { service: 'Service:101', startAt: at(hour), endAt: at(2 * hour), name: '時刻予約' },
      },
    },
    {
      name: 'update time reservation',
      request: TimeReservationDialogUpdateMutation,
      variables: { id: firstReservation, input: { enabled: true } },
    },
    { name: 'skipped duplicates', request: SkippedDuplicateListQuery, variables: {} },
    { name: 'margins', request: MarginFieldsQuery, variables: {} },
    { name: 'file name default', request: FileNameTemplateFieldDefaultQuery, variables: {} },
    { name: 'file name help', request: FileNameTemplateFieldHelpQuery, variables: {} },
    {
      name: 'file name preview',
      request: FileNameTemplateFieldPreviewQuery,
      variables: { template: '{{.Name}}', program: upcoming },
    },
    { name: 'watch reservations', request: WatchReservationListQuery, variables: {} },
    {
      name: 'watch reminder',
      request: WatchReminderSettingsMutation,
      variables: { minutesBefore: 10 },
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

  it('orders the services of a guide view first, and keeps the other services after them', async () => {
    const { executor } = newExecutor();
    const created = await executor.execute(text(GuideViewEditorCreateMutation), {
      input: { name: 'みかん', services: ['Service:1032'] },
    });
    const id = (created.data as { createGuideView: { id: string } }).createGuideView.id;

    const viewed = await executor.execute(text(guideQuery), {
      from: at(0),
      to: at(hour),
      view: id,
    });

    const ids = (viewed.data as { guide: { service: { id: string } }[] }).guide.map(
      (row) => row.service.id,
    );
    expect(ids[0]).toBe('Service:1032');
    expect(ids.slice(1, 3)).toEqual(['Service:1024', 'Service:1040']);
    expect(ids).toHaveLength(9);
  });

  it('returns search results in the order of broadcast across services', async () => {
    const { executor } = newExecutor();

    const result = await executor.execute(text(searchProgramsQuery), {
      conditions: {},
      archive: null,
      first: 20,
      after: null,
    });

    const nodes = (
      result.data as {
        searchPrograms: { edges: { node: { startAt: string; service: { id: string } } }[] };
      }
    ).searchPrograms.edges.map((edge) => edge.node);
    const starts = nodes.map((item) => Date.parse(item.startAt));
    expect(starts).toEqual(starts.toSorted((a, b) => a - b));
    expect(new Set(nodes.map((item) => item.service.id)).size).toBeGreaterThan(1);
  });

  it('marks only programs that have ended as archived in the guide', async () => {
    const { executor } = newExecutor();

    const result = await executor.execute(
      'query ($from: Time!, $to: Time!) { guide(from: $from, to: $to) { programs { endAt archived } } }',
      { from: at(-3 * hour), to: at(3 * hour) },
    );

    const items = (
      result.data as { guide: { programs: { endAt: string; archived: boolean }[] }[] }
    ).guide.flatMap((row) => row.programs);
    expect(items.some((item) => item.archived)).toBe(true);
    expect(items.every((item) => item.archived === Date.parse(item.endAt) <= now)).toBe(true);
  });
});

describe('demo watch reservation', () => {
  it('tells the other screens about the change', async () => {
    const { executor } = newExecutor();
    const results = (await executor.subscribe(text(DataChangesSubscription), {}))[
      Symbol.asyncIterator
    ]();
    const next = results.next();

    await executor.execute(text(WatchReserveButtonMutation), { program: upcoming, reserved: true });

    const { value } = await next;
    expect(value).toMatchObject({ data: { viewerEvents: { viewerSettingsVersion: 1 } } });
    await results.return?.();
  });

  it('refuses a program that has already started', async () => {
    const { executor } = newExecutor();

    const result = await executor.execute(text(WatchReserveButtonMutation), {
      program: past,
      reserved: true,
    });

    expect(result.errors?.[0]?.message).toBe('the program has already started');
  });
});
