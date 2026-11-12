import { act, cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react';
import { Environment, Network, RecordSource, Store, type GraphQLResponse } from 'relay-runtime';
import { afterEach, describe, expect, test, vi } from 'vitest';

import { createRootValue } from '../demo/data';
import { generatePrograms } from '../demo/epg';
import { createEventHub } from '../demo/events';
import { createExecutor } from '../demo/executor';

import { createDemoEnvironment, renderRoute } from '../test/route';
import { Route } from './reservations';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// rowOf は、予約の操作のボタンを含む、予約の行を返す。
function rowOf(button: HTMLElement): HTMLElement {
  return button.closest<HTMLElement>('[data-state]')!;
}

// firstRow は、操作できる先頭の予約の行を返す。
function firstRow(): HTMLElement {
  return rowOf(screen.getAllByRole('button', { name: '予約の操作' })[0]!);
}

describe('予約の一覧', () => {
  test('Kotatsu が同じ回を録画済みと分かって無効にした予約は、理由と先に録った局を出し、有効に戻すと理由を消す (#1500)', async () => {
    await renderRoute(Route, '/reservations', '/reservations', createDemoEnvironment());
    // デモでは、地上波の「こたつ探偵」の予約を、BS の局で先に録った同じ回と分かったものとして置いている。
    const reason = await screen.findByText(/^同じ回を.+で録画済みなので録画しない$/);
    const row = rowOf(reason);
    expect(row.dataset.disabled).toBe('true');
    expect(within(row).getByText('無効')).toBeDefined();

    fireEvent.click(within(row).getByRole('button', { name: '予約の操作' }));
    fireEvent.click(await screen.findByRole('menuitem', { name: '有効にする' }));

    await waitFor(() => expect(row.dataset.disabled).toBeUndefined());
    expect(screen.queryByText(/^同じ回を.+で録画済みなので録画しない$/)).toBeNull();
  });

  test('行を個別に無効にした後に、まとめて有効にすると、その行も有効の表示になる (#1425)', async () => {
    await renderRoute(Route, '/reservations', '/reservations', createDemoEnvironment());
    const actions = await screen.findAllByRole('button', { name: '予約の操作' });
    const row = rowOf(actions[0]!);
    expect(row.dataset.disabled).toBeUndefined();

    fireEvent.click(actions[0]!);
    fireEvent.click(await screen.findByRole('menuitem', { name: '無効にする' }));
    await waitFor(() => expect(firstRow().dataset.disabled).toBe('true'));

    fireEvent.click(screen.getByRole('button', { name: '選ぶ' }));
    fireEvent.click(within(firstRow()).getByRole('checkbox'));
    await act(async () =>
      fireEvent.click(screen.getByRole('button', { name: '選んだ1件を有効にする' })),
    );

    await waitFor(() => expect(firstRow().dataset.disabled).toBeUndefined());
  });

  test('続きで読んだ行を個別に無効にしてから、まとめて有効にすると、読み直した続きでも有効の表示になる (#1425)', async () => {
    // 1 ページ (25 件) に収まらないよう、デモの予約を 30 件足す。
    const now = Date.now();
    const executor = createExecutor(createRootValue(createEventHub(), now));
    const programs = generatePrograms(now)
      .filter((item) => item.start > now + 60 * 60_000)
      .slice(0, 30);
    for (const program of programs) {
      await executor.execute(
        'mutation ($program: ID!) { createProgramReservation(input: { program: $program }) { id } }',
        { program: program.id },
      );
    }
    const environment = new Environment({
      network: Network.create(
        async (request, variables) =>
          (await executor.execute(request.text ?? '', variables)) as GraphQLResponse,
      ),
      store: new Store(new RecordSource()),
    });
    await renderRoute(Route, '/reservations', '/reservations', environment);
    const loadMore = async () => {
      const before = screen.getAllByRole('button', { name: '予約の操作' }).length;
      fireEvent.click(await screen.findByRole('button', { name: 'さらに読み込む' }));
      await waitFor(() =>
        expect(screen.getAllByRole('button', { name: '予約の操作' }).length).toBeGreaterThan(
          before,
        ),
      );
    };
    const lastRow = () => rowOf(screen.getAllByRole('button', { name: '予約の操作' }).at(-1)!);
    await screen.findAllByRole('button', { name: '予約の操作' });
    await loadMore();

    fireEvent.click(within(lastRow()).getByRole('button', { name: '予約の操作' }));
    fireEvent.click(await screen.findByRole('menuitem', { name: '無効にする' }));
    await waitFor(() => expect(lastRow().dataset.disabled).toBe('true'));

    fireEvent.click(screen.getByRole('button', { name: '選ぶ' }));
    fireEvent.click(within(lastRow()).getByRole('checkbox'));
    await act(async () =>
      fireEvent.click(screen.getByRole('button', { name: '選んだ1件を有効にする' })),
    );
    await screen.findByRole('button', { name: 'さらに読み込む' });
    await loadMore();

    expect(lastRow().dataset.disabled).toBeUndefined();
  });
});
