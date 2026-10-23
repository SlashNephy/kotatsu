import { MantineProvider } from '@mantine/core';
import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, test, vi } from 'vitest';

import '../i18n';
import { CommentList } from './CommentList';

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
});
