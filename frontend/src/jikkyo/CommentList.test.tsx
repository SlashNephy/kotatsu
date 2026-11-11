import { MantineProvider, type MenuProps } from '@mantine/core';
import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, test, vi } from 'vitest';

import '../i18n';
import { CommentList, type CommentRow } from './CommentList';

// 行を描いた回数を数えるため、行ごとに 1 つ置くメニューを描いた回数を数える。
const menus = vi.hoisted(() => ({ count: 0 }));
vi.mock('@mantine/core', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@mantine/core')>();
  const Menu = (props: MenuProps) => {
    menus.count++;

    return <actual.Menu {...props} />;
  };

  return { ...actual, Menu: Object.assign(Menu, actual.Menu) };
});

const rowsUntil = (count: number): CommentRow[] =>
  Array.from({ length: count }, (_, i) => ({
    key: i,
    label: '21:00:00',
    content: `コメント${i}`,
    userId: `user${i % 3}`,
  }));

afterEach(() => {
  cleanup();
});

describe('CommentList', () => {
  test('行のメニューから、そのコメントを書いたユーザーをミュートする', async () => {
    const onMuteUser = vi.fn();
    render(
      <MantineProvider env="test">
        <CommentList
          rows={[{ key: 1, label: '21:00:00', content: 'わこつ', userId: 'abc' }]}
          onMuteUser={onMuteUser}
        />
      </MantineProvider>,
    );

    await userEvent.click(screen.getByRole('button', { name: '「わこつ」の操作' }));
    expect(screen.getByRole('menuitem', { name: 'ユーザー ID をコピー' })).toBeDefined();
    await userEvent.click(screen.getByRole('menuitem', { name: 'このユーザーをミュート' }));

    expect(onMuteUser).toHaveBeenCalledWith('abc');
  });

  test('ユーザー ID が分からないコメントは、コピーだけを出す', async () => {
    render(
      <MantineProvider env="test">
        <CommentList
          rows={[{ key: 1, label: '21:00:00', content: 'わこつ' }]}
          onMuteUser={() => {}}
        />
      </MantineProvider>,
    );

    await userEvent.click(screen.getByRole('button', { name: '「わこつ」の操作' }));
    expect(screen.getAllByRole('menuitem').map((item) => item.textContent)).toEqual(['コピー']);
  });

  test('行を足しても、前からある行は描き直さない', () => {
    const rows = rowsUntil(100);
    const { rerender } = render(
      <MantineProvider env="test">
        <CommentList rows={rows} onMuteUser={() => {}} />
      </MantineProvider>,
    );
    menus.count = 0;

    // 親が描き直すたびに onMuteUser は新しい関数になる。それでも、描くのは足した行だけにする。
    rerender(
      <MantineProvider env="test">
        <CommentList rows={[...rows, ...rowsUntil(101).slice(100)]} onMuteUser={() => {}} />
      </MantineProvider>,
    );

    expect(screen.getByText('コメント100')).toBeDefined();
    expect(menus.count).toBe(1);
  });

  test('frozen の間は、行が変わっても前の行のまま置き、解くと今の行を出す', () => {
    const { rerender } = render(
      <MantineProvider env="test">
        <CommentList rows={rowsUntil(1)} frozen />
      </MantineProvider>,
    );

    rerender(
      <MantineProvider env="test">
        <CommentList rows={rowsUntil(2)} frozen />
      </MantineProvider>,
    );
    expect(screen.queryByText('コメント1')).toBeNull();

    rerender(
      <MantineProvider env="test">
        <CommentList rows={rowsUntil(2)} />
      </MantineProvider>,
    );
    expect(screen.getByText('コメント1')).toBeDefined();
  });
});
