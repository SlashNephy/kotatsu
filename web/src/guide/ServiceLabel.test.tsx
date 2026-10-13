import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import { ServiceLabel } from './ServiceLabel';

afterEach(() => {
  cleanup();
});

describe('ServiceLabel', () => {
  test.each([
    { name: 'ロゴがあれば、名前の前に出す', logoUrl: '/media/services/1/logo', wantLogo: true },
    { name: 'ロゴがなければ、名前だけを出す', logoUrl: null, wantLogo: false },
  ])('$name', ({ logoUrl, wantLogo }) => {
    const { container } = render(<ServiceLabel name="こたつテスト1" logoUrl={logoUrl} />);

    expect(screen.getByText('こたつテスト1')).toBeDefined();
    expect(container.querySelector('img')?.getAttribute('src') ?? null).toBe(
      wantLogo ? logoUrl : null,
    );
  });

  test('ロゴを読めなければ、名前だけにし、URL が変われば読み直す', () => {
    const { container, rerender } = render(
      <ServiceLabel name="こたつテスト1" logoUrl="/media/services/1/logo" />,
    );

    fireEvent.error(container.querySelector('img')!);
    expect(container.querySelector('img')).toBeNull();
    expect(screen.getByText('こたつテスト1')).toBeDefined();

    rerender(<ServiceLabel name="こたつテスト2" logoUrl="/media/services/2/logo" />);
    expect(container.querySelector('img')?.getAttribute('src')).toBe('/media/services/2/logo');
  });
});
