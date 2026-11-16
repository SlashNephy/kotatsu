import { act, cleanup, fireEvent, screen, waitFor } from '@testing-library/react';
import { Environment, Network, RecordSource, Store, type GraphQLResponse } from 'relay-runtime';
import { afterEach, describe, expect, onTestFinished, test, vi } from 'vitest';

import { createRootValue } from '../../demo/data';
import { createEventHub } from '../../demo/events';
import { createExecutor } from '../../demo/executor';
import { createFailingEnvironment, renderRoute } from '../../test/route';
import { Route } from './$ruleId';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe('RulePage', () => {
  test('エラーの後に別のルールへ移ると、エラーの表示を消して読み直す (#1453)', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    // 1 つ目は読み込みに失敗する。2 つ目は、応答を返さず読み込み中のままにする。
    const { environment, requested } = createFailingEnvironment('Rule:1');
    const { router } = await renderRoute(Route, '/rules/$ruleId', '/rules/Rule:1', environment);
    expect(
      await screen.findByText('読み込めませんでした。時間をおいてやり直してください'),
    ).toBeTruthy();

    await act(() => router.navigate({ to: '/rules/$ruleId', params: { ruleId: 'Rule:2' } }));

    expect(screen.queryByText('読み込めませんでした。時間をおいてやり直してください')).toBeNull();
    expect(requested.some((variables) => variables.includes('Rule:2'))).toBe(true);
  });

  test('その場で新しいグループを作って選び、保存するとルールがそのグループに入る (#1493)', async () => {
    // jsdom には document.fonts がなく、メモの入力 (Textarea の autosize) が読むので、何もしないものを置く。
    Object.defineProperty(document, 'fonts', {
      configurable: true,
      value: { addEventListener: () => undefined, removeEventListener: () => undefined },
    });
    onTestFinished(() => {
      Reflect.deleteProperty(document, 'fonts');
    });
    const executor = createExecutor(createRootValue(createEventHub(), Date.now()));
    const rulesOf = async () =>
      (
        (await executor.execute('{ rules { id name group { name } } }', {})).data as {
          rules: { id: string; name: string; group: { name: string } | null }[];
        }
      ).rules;
    const target = (await rulesOf()).find((rule) => rule.name === 'こたつ探偵')!;
    const environment = new Environment({
      network: Network.create(
        async (request, variables) =>
          (await executor.execute(request.text ?? '', variables)) as GraphQLResponse,
      ),
      store: new Store(new RecordSource()),
    });
    await renderRoute(Route, '/rules/$ruleId', `/rules/${target.id}`, environment);

    fireEvent.click(await screen.findByRole('combobox', { name: 'グループ' }));
    await act(async () =>
      fireEvent.click(await screen.findByRole('option', { name: '新しいグループ…' })),
    );
    fireEvent.change(screen.getByRole('textbox', { name: '新しいグループの名前' }), {
      target: { value: '深夜アニメ' },
    });
    await act(async () => fireEvent.click(screen.getByRole('button', { name: 'グループを作る' })));
    await waitFor(() =>
      expect(screen.getByRole('combobox', { name: 'グループ' })).toHaveProperty(
        'value',
        '深夜アニメ',
      ),
    );
    // グループは作った時点で保存し、ルールは保存するまで変えない。
    expect((await rulesOf()).find((rule) => rule.id === target.id)?.group).toBeNull();

    await act(async () => fireEvent.click(screen.getByRole('button', { name: '保存' })));

    await waitFor(async () =>
      expect((await rulesOf()).find((rule) => rule.id === target.id)?.group).toEqual({
        name: '深夜アニメ',
      }),
    );
  });
});
