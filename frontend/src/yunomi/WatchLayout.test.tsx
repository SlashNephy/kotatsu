import { MantineProvider } from '@mantine/core';
import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import '../i18n';
import { WatchDescription } from './WatchLayout';

afterEach(() => {
  cleanup();
});

describe('WatchDescription', () => {
  test('説明の後に、番組の詳細の見出しと本文を続ける (#1646)', () => {
    render(
      <MantineProvider env="test">
        <WatchDescription
          text="都市伝説を回収する少年"
          extended={[
            { heading: '出演者', text: '【声】こたつ' },
            { heading: '', text: '' },
          ]}
        />
      </MantineProvider>,
    );

    expect(screen.getByText('都市伝説を回収する少年')).toBeTruthy();
    expect(screen.getByText('出演者')).toBeTruthy();
    expect(screen.getByText('【声】こたつ')).toBeTruthy();
  });

  test('説明が空でも、番組の詳細があれば出す', () => {
    render(
      <MantineProvider env="test">
        <WatchDescription text="" extended={[{ heading: '番組内容', text: 'みかんを食べる' }]} />
      </MantineProvider>,
    );

    expect(screen.getByText('番組内容')).toBeTruthy();
  });

  test('説明も番組の詳細も空なら、何も出さない', () => {
    const { container } = render(
      <MantineProvider env="test">
        <WatchDescription text=" " extended={[]} />
      </MantineProvider>,
    );

    expect(container.querySelector('.mantine-Spoiler-root')).toBeNull();
  });
});
