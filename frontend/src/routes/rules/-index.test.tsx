import { act, cleanup, fireEvent, screen, waitFor } from '@testing-library/react';
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
});

// setup は、デモのルール (こたつ探偵、縁側の午後) に、予約のないルールを 1 つ足した環境を作る。
// executor は、操作の後のルールを、画面を通さずに確かめるのに使う。
async function setup() {
  const executor = createExecutor(createRootValue(createEventHub(), Date.now()));
  await executor.execute('mutation ($input: RuleInput!) { createRule(input: $input) { id } }', {
    input: {
      name: '終わったアニメ',
      memo: 'ＦＩＮＡＬ',
      conditions: { keywords: [{ text: '放送の終わった番組', fields: ['NAME'] }] },
    },
  });
  const environment = new Environment({
    network: Network.create(
      async (request, variables) =>
        (await executor.execute(request.text ?? '', variables)) as GraphQLResponse,
    ),
    store: new Store(new RecordSource()),
  });
  const enabledOf = async () => {
    const result = await executor.execute('{ rules { name enabled } }', {});

    return Object.fromEntries(
      (result.data as { rules: { name: string; enabled: boolean }[] }).rules.map((rule) => [
        rule.name,
        rule.enabled,
      ]),
    );
  };

  return { environment, enabledOf };
}

// shownRules は、一覧に出しているルールの名前を、出している順に返す。
function shownRules(): string[] {
  return screen
    .queryAllByRole('link')
    .filter((link) => /\/rules\/Rule/.test(link.getAttribute('href') ?? ''))
    .map((link) => link.textContent ?? '');
}

// choose は、名前が label の選択肢の入力を開き、option を選ぶ。
async function choose(label: string, option: string) {
  fireEvent.click(screen.getByRole('combobox', { name: label }));
  await act(async () => fireEvent.click(await screen.findByRole('option', { name: option })));
}

describe('ルールの一覧', () => {
  test('URL の語で、全角で書いたメモのルールを、半角の語から見つける (#1493)', async () => {
    const { environment } = await setup();
    await renderRoute(Route, '/rules/', '/rules?q=final', environment);

    await waitFor(() => expect(shownRules()).toEqual(['終わったアニメ']));
    expect(screen.getByRole('textbox', { name: '名前、メモ、キーワードで探す' })).toHaveProperty(
      'value',
      'final',
    );
    expect(screen.getByText('3件中1件')).toBeDefined();
  });

  test('打った語で絞り、URL に残す。合うものがなければ、絞り込みを解いて全部に戻せる', async () => {
    const { environment } = await setup();
    const { router } = await renderRoute(Route, '/rules/', '/rules', environment);
    await waitFor(() => expect(shownRules()).toHaveLength(3));

    fireEvent.change(screen.getByRole('textbox', { name: '名前、メモ、キーワードで探す' }), {
      target: { value: 'こたつ' },
    });
    await waitFor(() => expect(shownRules()).toEqual(['こたつ探偵']));
    expect(router.state.location.search).toEqual({ q: 'こたつ' });

    fireEvent.change(screen.getByRole('textbox', { name: '名前、メモ、キーワードで探す' }), {
      target: { value: 'どれにもない語' },
    });
    expect(await screen.findByText('条件に合うルールはありません')).toBeDefined();

    fireEvent.click(screen.getByRole('button', { name: '絞り込みを解除' }));
    await waitFor(() => expect(shownRules()).toHaveLength(3));
    expect(router.state.location.search).toEqual({});
  });

  test('入力の外で URL の語が消えたら、入力も空にして全部を出す', async () => {
    const { environment } = await setup();
    const { router } = await renderRoute(Route, '/rules/', '/rules?q=こたつ', environment);
    await waitFor(() => expect(shownRules()).toEqual(['こたつ探偵']));

    // 上のメニューの「ルール」から開き直したときと同じく、クエリのない /rules へ移る。
    await act(() => router.navigate({ to: '/rules', search: {} }));

    await waitFor(() => expect(shownRules()).toHaveLength(3));
    expect(screen.getByRole('textbox', { name: '名前、メモ、キーワードで探す' })).toHaveProperty(
      'value',
      '',
    );
  });

  test('視聴ルールのタブへ移ると、探す語と絞り込みと並べ替えを URL から外す', async () => {
    const { environment } = await setup();
    const { router } = await renderRoute(
      Route,
      '/rules/',
      '/rules?q=こたつ&state=enabled&sort=name&order=desc',
      environment,
    );
    await waitFor(() => expect(shownRules()).toEqual(['こたつ探偵']));

    fireEvent.click(screen.getByRole('tab', { name: '視聴ルール' }));

    await waitFor(() => expect(router.state.location.search).toEqual({ watch: true }));
  });

  test('並べ替えを URL から読み、向きを変えられる', async () => {
    const { environment } = await setup();
    const { router } = await renderRoute(Route, '/rules/', '/rules?sort=name', environment);

    await waitFor(() =>
      expect(shownRules()).toEqual(['こたつ探偵', '縁側の午後', '終わったアニメ']),
    );

    fireEvent.click(screen.getByRole('button', { name: '昇順' }));
    await waitFor(() =>
      expect(shownRules()).toEqual(['終わったアニメ', '縁側の午後', 'こたつ探偵']),
    );
    expect(router.state.location.search).toEqual({ sort: 'name', order: 'desc' });
  });

  test('まとめての操作は、絞り込みで隠れた、選んだルールには及ばない (#1493)', async () => {
    const { environment, enabledOf } = await setup();
    await renderRoute(Route, '/rules/', '/rules', environment);
    await waitFor(() => expect(shownRules()).toHaveLength(3));

    // 予約のあるルールを先に選んでから、予約のないルールに絞る。
    fireEvent.click(screen.getByRole('button', { name: '選ぶ' }));
    fireEvent.click(screen.getByRole('checkbox', { name: 'こたつ探偵' }));
    await choose('予約で絞り込む', 'これからの予約なし');
    await waitFor(() => expect(shownRules()).toEqual(['終わったアニメ']));
    expect(screen.getByRole('button', { name: '選んだ0件を無効にする' })).toHaveProperty(
      'disabled',
      true,
    );

    fireEvent.click(screen.getByRole('button', { name: '表示中の1件を選ぶ' }));
    await act(async () =>
      fireEvent.click(screen.getByRole('button', { name: '選んだ1件を無効にする' })),
    );

    await waitFor(async () =>
      expect(await enabledOf()).toEqual({
        こたつ探偵: true,
        縁側の午後: true,
        終わったアニメ: false,
      }),
    );
  });
});
