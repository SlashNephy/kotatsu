import { describe, expect, it } from 'vitest';

import { createRootValue } from './data';
import { generatePrograms } from './epg';
import { createEventHub } from './events';
import { createExecutor } from './executor';
import { renderFileName, sampleFileNameData } from './filename';

const now = Date.parse('2026-10-19T12:00:00Z');
const hour = 60 * 60 * 1000;
const at = (offset: number) => new Date(now + offset).toISOString();

function newExecutor() {
  const events = createEventHub();

  return { events, executor: createExecutor(createRootValue(events, now)) };
}

type ReservationNode = {
  id: string;
  kind: string;
  name: string;
  enabled: boolean;
  assignment: { state: string; tuner: number | null };
};

async function reservations(executor: ReturnType<typeof newExecutor>['executor']) {
  const result = await executor.execute(
    `query ($from: Time!, $to: Time!) {
      reservations(from: $from, to: $to, first: 100) {
        edges { node { id kind name enabled assignment { state tuner } } }
      }
    }`,
    { from: at(0), to: at(8 * 24 * hour) },
  );
  expect(result.errors).toBeUndefined();

  return (
    result.data as { reservations: { edges: { node: ReservationNode }[] } }
  ).reservations.edges.map((edge) => edge.node);
}

describe('demo rules', () => {
  it('reserves the programs that match a new rule, and forgets them when the rule is deleted', async () => {
    const { executor } = newExecutor();

    const created = await executor.execute(
      'mutation ($input: RuleInput!) { createRule(input: $input) { id upcomingReservationCount } }',
      {
        input: { name: '編み物', conditions: { keywords: [{ text: '編み物', fields: ['NAME'] }] } },
      },
    );
    const rule = (created.data as { createRule: { id: string; upcomingReservationCount: number } })
      .createRule;
    expect(rule.upcomingReservationCount).toBeGreaterThan(0);
    const reserved = (await reservations(executor)).filter((item) => item.name.includes('編み物'));
    expect(reserved.length).toBeGreaterThan(0);
    expect(reserved.every((item) => item.kind === 'RULE')).toBe(true);

    await executor.execute('mutation ($id: ID!) { deleteRule(id: $id) }', { id: rule.id });

    expect((await reservations(executor)).filter((item) => item.name.includes('編み物'))).toEqual(
      [],
    );
  });

  it('disables a rule reservation instead of deleting it, so the rule does not reserve it again', async () => {
    const { executor } = newExecutor();
    const target = (await reservations(executor)).find((item) => item.kind === 'RULE')!;

    const result = await executor.execute(
      'mutation ($id: ID!) { deleteReservation(id: $id) { id reservation { id enabled } } }',
      { id: target.id },
    );

    expect(result.data).toEqual({
      deleteReservation: { id: target.id, reservation: { id: target.id, enabled: false } },
    });
    const after = (await reservations(executor)).filter((item) => item.name === target.name);
    expect(after.find((item) => item.id === target.id)?.enabled).toBe(false);
  });

  it('previews only programs that have not started', async () => {
    const { executor } = newExecutor();

    const result = await executor.execute(
      `query ($input: RuleInput!) { previewRule(input: $input, first: 100) { edges { node { startAt } } } }`,
      { input: { name: 'ニュース', conditions: { genres: [{ lv1: 0 }] } } },
    );

    const starts = (
      result.data as { previewRule: { edges: { node: { startAt: string } }[] } }
    ).previewRule.edges.map((edge) => Date.parse(edge.node.startAt));
    expect(starts.length).toBeGreaterThan(0);
    expect(starts.every((start) => start > now)).toBe(true);
  });
});

describe('demo rules and their reservations', () => {
  it('keeps a cancelled rule reservation cancelled after the rule is disabled and enabled again', async () => {
    const { executor } = newExecutor();
    const before = await reservations(executor);
    const target = before.find((item) => item.kind === 'RULE')!;
    const rules = await executor.execute('{ rules { id name } }', {});
    const rule = (rules.data as { rules: { id: string; name: string }[] }).rules.find((item) =>
      target.name.includes(item.name),
    )!;

    await executor.execute('mutation ($id: ID!) { deleteReservation(id: $id) { id } }', {
      id: target.id,
    });
    for (const enabled of [false, true]) {
      await executor.execute(
        'mutation ($id: ID!, $enabled: Boolean!) { setRuleEnabled(id: $id, enabled: $enabled) { id } }',
        {
          id: rule.id,
          enabled,
        },
      );
    }

    // 同じ名前の番組がほかの局にもあるので、名前の一致する予約の数が変わらないことで、作り直していないことを確かめる。
    const after = await reservations(executor);
    const sameName = (list: ReservationNode[]) => list.filter((item) => item.name === target.name);
    expect(sameName(after)).toHaveLength(sameName(before).length);
    expect(after.find((item) => item.id === target.id)?.enabled).toBe(false);
  });
});

describe('demo reservations', () => {
  it('reports a conflict when more programs overlap than there are tuners', async () => {
    const { executor } = newExecutor();
    // 地上波のチューナーは 2 台なので、同じ時間帯に 3 つ予約すると、1 つは録れない。
    const start = now + 30 * 24 * hour;
    for (const service of ['Service:1024', 'Service:1032', 'Service:1040']) {
      await executor.execute(
        'mutation ($input: TimeReservationInput!) { createTimeReservation(input: $input) { id } }',
        {
          input: {
            service,
            startAt: new Date(start).toISOString(),
            endAt: new Date(start + hour).toISOString(),
          },
        },
      );
    }

    const result = await executor.execute(
      `query ($from: Time!, $to: Time!) {
        reservations(from: $from, to: $to, first: 10) { edges { node { assignment { state tuner } } } }
      }`,
      { from: new Date(start).toISOString(), to: new Date(start + hour).toISOString() },
    );

    const states = (
      result.data as { reservations: { edges: { node: { assignment: { state: string } } }[] } }
    ).reservations.edges.map((edge) => edge.node.assignment.state);
    expect(states).toEqual(['OK', 'OK', 'CONFLICT']);
  });

  it('returns the existing reservation when the same program is reserved twice', async () => {
    const { executor } = newExecutor();
    const program = generatePrograms(now).find(
      (item) => item.start > now && item.name.startsWith('はんてん旅'),
    )!;
    const reserve = () =>
      executor.execute(
        'mutation ($input: ProgramReservationInput!) { createProgramReservation(input: $input) { id } }',
        {
          input: { program: program.id },
        },
      );

    const first = await reserve();
    const second = await reserve();

    expect(second.data).toEqual(first.data);
  });

  it('tells the screens that reservations changed', async () => {
    const { events, executor } = newExecutor();
    const received: unknown[] = [];
    const listening = events.listen((event) => event.dataChange);
    const collect = (async () => {
      for await (const change of listening) {
        received.push(change);
      }
    })();

    await executor.execute(
      'mutation ($input: TimeReservationInput!) { createTimeReservation(input: $input) { id } }',
      { input: { service: 'Service:101', startAt: at(hour), endAt: at(2 * hour) } },
    );
    // 届いた出来事を読み終えてから、購読をやめる。
    await new Promise((resolve) => setTimeout(resolve, 0));
    await listening.return?.();
    await collect;

    expect(received).toEqual([
      { channel: 'RESERVATIONS', version: 1 },
      { channel: 'ASSIGNMENTS', version: 1 },
    ]);
  });
});

describe('demo file name template', () => {
  it('fills the fields that the help lists', () => {
    expect(
      renderFileName(
        '[{{.ChannelType}}_{{half .ServiceName}}]{{.StartAt.Format "06-01-02"}}({{weekdayJa .StartAt}})',
        sampleFileNameData,
      ),
    ).toEqual({
      fileName: '[GR_KOTATSU TV1]26-10-12(月)',
      error: null,
    });
  });

  it('reports a syntax that the demo does not support', () => {
    expect(renderFileName('{{.Description}}', sampleFileNameData).error).toContain(
      '{{.Description}}',
    );
  });
});
