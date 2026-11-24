import { act, cleanup, fireEvent, screen, waitFor } from '@testing-library/react';
import type { Variables } from 'relay-runtime';
import { afterEach, describe, expect, test, vi } from 'vitest';

import { createTestEnvironment, renderRoute } from '../../test/route';
import { Route } from './captions';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const md5a = '0123456789abcdef0123456789abcdef';
const md5b = 'fedcba9876543210fedcba9876543210';

// serve は、サーバーの外字のテーブルを持ち、保存されたら送った外字を当てる環境を作る。sent は、送った外字の変更。
function serve() {
  let table = [{ md5: md5a, text: 'あ' }];
  const sent: Variables[] = [];
  const environment = createTestEnvironment((request, variables) => {
    if (request.name === 'captionsMutation') {
      const changes = (
        variables.input as { drcsReplacements: { md5: string; text: string | null }[] }
      ).drcsReplacements;
      sent.push(changes);
      for (const change of changes) {
        table = table.filter((row) => row.md5 !== change.md5);
        if (change.text !== null) {
          table = [...table, { md5: change.md5, text: change.text }];
        }
      }

      return { data: { updateSystemSettings: { updatedAt: '2026-11-11T00:00:01Z' } } };
    }

    return {
      data: {
        systemSettings: {
          drcsReplacements: table.map((row) => ({ ...row })),
          updatedAt: '2026-11-11T00:00:00Z',
        },
      },
    };
  });

  return { environment, sent };
}

describe('字幕の外字の設定 (#1740)', () => {
  test('置き換える文字は、欄を離れたら、その外字だけを保存する', async () => {
    const { environment, sent } = serve();
    await renderRoute(Route, '/admin/captions', '/admin/captions', environment);
    const text = await screen.findByDisplayValue('あ');

    fireEvent.change(text, { target: { value: 'い' } });
    expect(sent).toEqual([]);
    await act(async () => fireEvent.blur(text));
    await screen.findByRole('status');

    expect(sent).toEqual([[{ md5: md5a, text: 'い' }]]);
  });

  test('長すぎる文字は、理由を出して保存しない', async () => {
    const { environment, sent } = serve();
    await renderRoute(Route, '/admin/captions', '/admin/captions', environment);
    const text = await screen.findByDisplayValue('あ');

    fireEvent.change(text, { target: { value: 'あいうえお' } });
    await act(async () => fireEvent.blur(text));

    expect(screen.getByText('1〜4 文字で入れてください')).toBeDefined();
    expect(sent).toEqual([]);
  });

  test('足すと消すは、押したらすぐ、その外字だけを保存する', async () => {
    const { environment, sent } = serve();
    await renderRoute(Route, '/admin/captions', '/admin/captions', environment);
    await screen.findByDisplayValue('あ');

    fireEvent.change(screen.getByRole('textbox', { name: 'MD5' }), { target: { value: md5b } });
    fireEvent.change(screen.getAllByRole('textbox', { name: '置き換える文字' }).at(-1)!, {
      target: { value: 'う' },
    });
    await act(async () => fireEvent.click(screen.getByRole('button', { name: '足す' })));
    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0]).toEqual([{ md5: md5b, text: 'う' }]);

    await act(async () => fireEvent.click(screen.getAllByRole('button', { name: '消す' })[0]!));
    await waitFor(() => expect(sent).toHaveLength(2));
    expect(sent[1]).toEqual([{ md5: md5a, text: null }]);
  });
});
