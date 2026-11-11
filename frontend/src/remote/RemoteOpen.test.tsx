import { act, cleanup, fireEvent, screen } from '@testing-library/react';
import type { GraphQLResponse } from 'relay-runtime';
import { afterEach, describe, expect, test, vi } from 'vitest';

import { createTestEnvironment, renderWithRouter } from '../test/route';
import { RemoteOpen } from './RemoteOpen';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// recordings は、名前 name の録画を 1 つ返す応答。
function recordings(name: string): GraphQLResponse {
  return {
    data: {
      recordings: {
        edges: [
          {
            node: {
              id: `Recording:${name}`,
              name,
              startAt: '2026-11-11T00:00:00Z',
              service: { name: 'こたつテレビ' },
            },
          },
        ],
      },
    },
  };
}

describe('RemoteOpen の録画の検索 (#1423)', () => {
  test('前の検索の応答が後から届いても、最後の検索の結果を出し、読み込み中を消さない', async () => {
    // pending は、検索の言葉ごとの、応答を返す関数。
    const pending = new Map<string, (response: GraphQLResponse) => void>();
    const environment = createTestEnvironment((request, variables) => {
      if (request.name === 'RemoteOpenServicesQuery') {
        return { data: { services: [] } };
      }

      return new Promise((resolve) => pending.set(String(variables.keyword), resolve));
    });
    await renderWithRouter(<RemoteOpen onOpen={vi.fn()} />, environment);
    fireEvent.click(await screen.findByText('録画'));
    const input = await screen.findByRole('textbox', { name: '番組名で録画を探す' });

    // A を探し、応答を待つ間に B を探す。読み込み中も、Enter で送れる。
    fireEvent.change(input, { target: { value: 'A' } });
    fireEvent.submit(input.closest('form')!);
    fireEvent.change(input, { target: { value: 'B' } });
    fireEvent.submit(input.closest('form')!);

    // A の応答が先に届いても、B を待っている間は読み込み中のまま。
    await act(async () => pending.get('A')!(recordings('A の録画')));
    expect(screen.queryByText('A の録画')).toBeNull();
    expect(screen.queryByRole('button', { name: '探す' })).toBeNull();

    await act(async () => pending.get('B')!(recordings('B の録画')));
    expect(screen.getByText('B の録画')).toBeTruthy();
    expect(screen.getByRole('button', { name: '探す' })).toBeTruthy();
  });

  test('B の応答の後に A の応答が届いても、B の結果のまま', async () => {
    const pending = new Map<string, (response: GraphQLResponse) => void>();
    const environment = createTestEnvironment((request, variables) => {
      if (request.name === 'RemoteOpenServicesQuery') {
        return { data: { services: [] } };
      }

      return new Promise((resolve) => pending.set(String(variables.keyword), resolve));
    });
    await renderWithRouter(<RemoteOpen onOpen={vi.fn()} />, environment);
    fireEvent.click(await screen.findByText('録画'));
    const input = await screen.findByRole('textbox', { name: '番組名で録画を探す' });
    fireEvent.change(input, { target: { value: 'A' } });
    fireEvent.submit(input.closest('form')!);
    fireEvent.change(input, { target: { value: 'B' } });
    fireEvent.submit(input.closest('form')!);

    await act(async () => pending.get('B')!(recordings('B の録画')));
    await act(async () => pending.get('A')!(recordings('A の録画')));

    expect(screen.getByText('B の録画')).toBeTruthy();
    expect(screen.queryByText('A の録画')).toBeNull();
  });
});
