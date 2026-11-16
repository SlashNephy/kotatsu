import { act, cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react';
import { Environment, Network, RecordSource, Store, type GraphQLResponse } from 'relay-runtime';
import { afterEach, describe, expect, test, vi } from 'vitest';

import { createRootValue } from '../../demo/data';
import { createEventHub } from '../../demo/events';
import { createExecutor } from '../../demo/executor';

import { renderRoute } from '../../test/route';
import { Route } from './index';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  localStorage.clear();
});

type Executor = ReturnType<typeof createExecutor>;

// run は、画面を通さずに、デモのサーバーへ操作を送り、data を返す。
async function run<T>(executor: Executor, query: string, variables = {}): Promise<T> {
  const result = await executor.execute(query, variables);
  expect(result.errors).toBeUndefined();

  return result.data as T;
}

// setup は、デモのルール (こたつ探偵、縁側の午後) に、ルールを 1 つ足し、グループ「アニメ」と「空のグループ」を作った環境を作る。
// こたつ探偵と終わったアニメはアニメに入れ、縁側の午後はどのグループにも入れない。
// role を VIEWER にすると、視聴者として開く。
async function setup(role: 'ADMIN' | 'VIEWER' = 'ADMIN') {
  const executor = createExecutor(createRootValue(createEventHub(), Date.now()));
  const { createRuleGroup: anime } = await run<{ createRuleGroup: { id: string } }>(
    executor,
    'mutation { createRuleGroup(name: "アニメ") { id } }',
  );
  await run(executor, 'mutation { createRuleGroup(name: "空のグループ") { id } }');
  await run(executor, 'mutation ($input: RuleInput!) { createRule(input: $input) { id } }', {
    input: {
      name: '終わったアニメ',
      conditions: { keywords: [{ text: '放送の終わった番組', fields: ['NAME'] }] },
      groupId: anime.id,
    },
  });
  const { rules } = await run<{ rules: { id: string; name: string }[] }>(
    executor,
    '{ rules { id name } }',
  );
  await run(
    executor,
    'mutation ($ids: [ID!]!, $groupId: ID) { setRulesGroup(ids: $ids, groupId: $groupId) { processed } }',
    {
      ids: rules.filter((rule) => rule.name === 'こたつ探偵').map((rule) => rule.id),
      groupId: anime.id,
    },
  );
  const environment = new Environment({
    network: Network.create(async (request, variables) => {
      const result = (await executor.execute(request.text ?? '', variables)) as GraphQLResponse & {
        data?: { viewer?: { role: string } };
      };
      if (result.data?.viewer !== undefined) {
        result.data.viewer.role = role;
      }

      return result;
    }),
    store: new Store(new RecordSource()),
  });
  // groupsOf は、ルールの名前と、入っているグループの名前 (なければ null) を返す。
  const groupsOf = async () => {
    const data = await run<{ rules: { name: string; group: { name: string } | null }[] }>(
      executor,
      '{ rules { name group { name } } }',
    );

    return Object.fromEntries(data.rules.map((rule) => [rule.name, rule.group?.name ?? null]));
  };

  return { executor, environment, groupsOf };
}

// layout は、一覧の見出しと、その下に出しているルールの名前を、出している順に返す。
function layout(): [string, string[]][] {
  return screen.getAllByRole('heading', { level: 3 }).map((heading) => {
    const section = heading.closest('.mantine-Stack-root')!;
    const rules = within(section as HTMLElement)
      .queryAllByRole('link')
      .filter((link) => /\/rules\/Rule/.test(link.getAttribute('href') ?? ''))
      .map((link) => link.textContent ?? '');

    return [heading.textContent?.replace(/\d+$/, '') ?? '', rules];
  });
}

// choose は、名前が label の選択肢の入力を開き、option を選ぶ。
async function choose(label: string, option: string) {
  fireEvent.click(screen.getByRole('combobox', { name: label }));
  await act(async () => fireEvent.click(await screen.findByRole('option', { name: option })));
}

describe('ルールの一覧のグループ (#1493)', () => {
  test('グループの順に見出しを分け、空のグループも出し、「その他」を最後に置く。並べ替えはグループの中で効く', async () => {
    const { environment } = await setup();
    await renderRoute(Route, '/rules/', '/rules?sort=name&order=desc', environment);

    await waitFor(() =>
      expect(layout()).toEqual([
        ['アニメ', ['終わったアニメ', 'こたつ探偵']],
        ['空のグループ', []],
        ['その他', ['縁側の午後']],
      ]),
    );
  });

  test('絞り込むと、当たるルールのないグループの見出しを出さない', async () => {
    const { environment } = await setup();
    await renderRoute(Route, '/rules/', '/rules?q=縁側', environment);

    await waitFor(() => expect(layout()).toEqual([['その他', ['縁側の午後']]]));
  });

  test('見出しを押すと折りたたみ、折りたたんだルールはまとめての操作の対象にしない。絞り込むと開く', async () => {
    const { environment } = await setup();
    const { router } = await renderRoute(Route, '/rules/', '/rules', environment);
    await waitFor(() => expect(layout()).toHaveLength(3));

    fireEvent.click(screen.getByRole('button', { name: /^アニメ \d+$/ }));
    expect(screen.getByRole('button', { name: /^アニメ \d+$/ }).getAttribute('aria-expanded')).toBe(
      'false',
    );
    expect(layout()[0]).toEqual(['アニメ', []]);

    fireEvent.click(screen.getByRole('button', { name: '選ぶ' }));
    expect(screen.getByRole('button', { name: '表示中の1件を選ぶ' })).toBeDefined();

    // 探す語で絞ると、折りたたんだ見出しも開いて、当たったルールを出す。
    await act(() => router.navigate({ to: '/rules', search: { q: 'こたつ' } }));
    await waitFor(() => expect(layout()).toEqual([['アニメ', ['こたつ探偵']]]));
  });

  test('選んだルールを、まとめてグループへ移す', async () => {
    const { environment, groupsOf } = await setup();
    await renderRoute(Route, '/rules/', '/rules', environment);
    await waitFor(() => expect(layout()).toHaveLength(3));

    fireEvent.click(screen.getByRole('button', { name: '選ぶ' }));
    fireEvent.click(screen.getByRole('checkbox', { name: 'こたつ探偵' }));
    fireEvent.click(screen.getByRole('checkbox', { name: '縁側の午後' }));
    fireEvent.click(screen.getByRole('button', { name: '選んだ2件をグループへ移す' }));
    await choose('移す先', '空のグループ');
    const dialog = await screen.findByRole('dialog', { name: 'ルールをグループへ移す' });
    await act(async () =>
      fireEvent.click(within(dialog).getByRole('button', { name: '選んだ2件をグループへ移す' })),
    );

    expect(await screen.findByText('2件のルールを移しました')).toBeDefined();
    expect(await groupsOf()).toEqual({
      こたつ探偵: '空のグループ',
      縁側の午後: '空のグループ',
      終わったアニメ: 'アニメ',
    });
    await waitFor(() =>
      expect(layout()).toEqual([
        ['アニメ', ['終わったアニメ']],
        ['空のグループ', ['こたつ探偵', '縁側の午後']],
      ]),
    );
  });

  test('グループの名前を変え、並べ替え、消すと、中のルールは「その他」に戻る', async () => {
    const { environment, groupsOf } = await setup();
    await renderRoute(Route, '/rules/', '/rules', environment);
    await waitFor(() => expect(layout()).toHaveLength(3));

    fireEvent.click(screen.getByRole('button', { name: 'アニメの操作' }));
    fireEvent.click(await screen.findByRole('menuitem', { name: '名前を変える' }));
    const renameDialog = await screen.findByRole('dialog', { name: 'グループの名前を変える' });
    fireEvent.change(within(renameDialog).getByRole('textbox', { name: 'グループの名前' }), {
      target: { value: '深夜アニメ' },
    });
    await act(async () =>
      fireEvent.click(within(renameDialog).getByRole('button', { name: '名前を変える' })),
    );
    await waitFor(() => expect(layout()[0]?.[0]).toBe('深夜アニメ'));

    fireEvent.click(screen.getByRole('button', { name: '深夜アニメの操作' }));
    await act(async () =>
      fireEvent.click(await screen.findByRole('menuitem', { name: '下へ移す' })),
    );
    await waitFor(() =>
      expect(layout().map(([name]) => name)).toEqual(['空のグループ', '深夜アニメ', 'その他']),
    );

    fireEvent.click(screen.getByRole('button', { name: '深夜アニメの操作' }));
    fireEvent.click(await screen.findByRole('menuitem', { name: 'グループを削除' }));
    const deleteDialog = await screen.findByRole('dialog', { name: 'グループを削除' });
    await act(async () =>
      fireEvent.click(within(deleteDialog).getByRole('button', { name: 'グループを削除' })),
    );

    await waitFor(() =>
      expect(layout()).toEqual([
        ['空のグループ', []],
        ['その他', ['こたつ探偵', '縁側の午後', '終わったアニメ']],
      ]),
    );
    expect(await groupsOf()).toEqual({ こたつ探偵: null, 縁側の午後: null, 終わったアニメ: null });
  });

  test('同じ名前のグループは作れず、そのことを示す', async () => {
    const { environment } = await setup();
    await renderRoute(Route, '/rules/', '/rules', environment);
    await waitFor(() => expect(layout()).toHaveLength(3));

    fireEvent.click(screen.getByRole('button', { name: 'グループを作る' }));
    const dialog = await screen.findByRole('dialog', { name: 'グループを作る' });
    fireEvent.change(within(dialog).getByRole('textbox', { name: 'グループの名前' }), {
      target: { value: 'アニメ' },
    });
    await act(async () =>
      fireEvent.click(within(dialog).getByRole('button', { name: 'グループを作る' })),
    );

    expect(
      await within(dialog).findByText('空の名前と、ほかのグループと同じ名前にはできません'),
    ).toBeDefined();
  });

  test('視聴者には、グループを作る、変える、移す操作を出さない', async () => {
    const { environment } = await setup('VIEWER');
    await renderRoute(Route, '/rules/', '/rules', environment);
    await waitFor(() => expect(layout()).toHaveLength(3));

    expect(screen.queryByRole('button', { name: 'グループを作る' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'アニメの操作' })).toBeNull();
    expect(screen.queryByRole('button', { name: '選ぶ' })).toBeNull();
  });
});
