import { act, cleanup, fireEvent, screen } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import { createDemoEnvironment, renderWithRouter } from '../test/route';
import { HeaderSearch } from './HeaderSearch';

afterEach(() => {
  cleanup();
});

// search は、虫眼鏡を押して、text を打って Enter を押す。
async function search(text: string) {
  const { router } = await renderWithRouter(<HeaderSearch />, createDemoEnvironment());
  fireEvent.click(screen.getByRole('button', { name: '番組検索' }));
  const input = await screen.findByRole('textbox', { name: '番組名で探す' });
  fireEvent.change(input, { target: { value: text } });
  await act(async () => {
    fireEvent.submit(input);
  });

  return router;
}

describe('HeaderSearch (#1627)', () => {
  test('打った言葉を番組名のキーワードにして、過去の放送を含めずに番組検索の画面へ移る', async () => {
    const router = await search('  ニュース  ');

    expect(router.state.location.pathname).toBe('/search');
    expect(router.state.location.state.searchArchive).toBe(false);
    expect(router.state.location.state.searchConditions?.keywords).toEqual([
      { text: 'ニュース', regexp: false, caseSensitive: false, fields: ['NAME'] },
    ]);
    // 移ったら、入力欄を閉じる。
    expect(screen.queryByRole('textbox', { name: '番組名で探す' })).toBeNull();
  });

  test('何も打たずに Enter を押したら、条件を入れずに番組検索の画面を開く', async () => {
    const router = await search(' ');

    expect(router.state.location.pathname).toBe('/search');
    expect(router.state.location.state.searchConditions).toBeUndefined();
    expect(router.state.location.state.searchArchive).toBeUndefined();
  });
});
