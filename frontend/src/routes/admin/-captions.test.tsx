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

describe('字幕の外字の設定', () => {
  test('保存の後は「保存」を押せず、次の保存では、そのあとに変えた外字だけを送る (#1419)', async () => {
    // table は、サーバーの外字の表。保存すると、送った差分を当てる。
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
    await renderRoute(Route, '/admin/captions', '/admin/captions', environment);
    const save = () => screen.getByRole('button', { name: '保存' });

    fireEvent.change(await screen.findByDisplayValue('あ'), { target: { value: 'い' } });
    await act(async () => fireEvent.click(save()));
    await waitFor(() => expect(screen.getByRole('status')).toBeTruthy());

    expect(sent).toEqual([[{ md5: md5a, text: 'い' }]]);
    await waitFor(() => expect(save()).toHaveProperty('disabled', true));

    fireEvent.change(screen.getByRole('textbox', { name: 'MD5' }), { target: { value: md5b } });
    fireEvent.change(screen.getAllByRole('textbox', { name: '置き換える文字' }).at(-1)!, {
      target: { value: 'う' },
    });
    fireEvent.click(screen.getByRole('button', { name: '足す' }));
    await act(async () => fireEvent.click(save()));

    await waitFor(() => expect(sent).toHaveLength(2));
    expect(sent[1]).toEqual([{ md5: md5b, text: 'う' }]);
  });
});
