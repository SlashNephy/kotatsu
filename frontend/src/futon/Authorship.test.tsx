import { MantineProvider } from '@mantine/core';
import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import '../i18n';
import { Authorship } from './Authorship';

afterEach(() => {
  cleanup();
});

describe('Authorship', () => {
  test.each([
    {
      name: 'ユーザーがいれば、表示名を出す',
      createdBy: { displayName: 'あかり' },
      updatedBy: { displayName: 'ひなた' },
      automatic: false,
      want: ['作成: あかり・2026/10/16 09:00', '更新: ひなた・2026/10/17 21:30'],
    },
    {
      name: 'ユーザーが消えていれば、削除されたユーザーと出す',
      createdBy: null,
      updatedBy: { displayName: 'ひなた' },
      automatic: false,
      want: ['作成: 削除されたユーザー・2026/10/16 09:00', '更新: ひなた・2026/10/17 21:30'],
    },
    {
      name: 'neko が作ったルール予約は、自動と出す',
      createdBy: null,
      updatedBy: null,
      automatic: true,
      want: ['作成: 自動・2026/10/16 09:00', '更新: 自動・2026/10/17 21:30'],
    },
  ])('$name', ({ createdBy, updatedBy, automatic, want }) => {
    render(
      <MantineProvider>
        <Authorship
          createdAt="2026-10-16T00:00:00Z"
          createdBy={createdBy}
          updatedAt="2026-10-17T12:30:00Z"
          updatedBy={updatedBy}
          automatic={automatic}
        />
      </MantineProvider>,
    );

    for (const text of want) {
      expect(screen.getByText(text)).toBeDefined();
    }
  });
});
