import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import { ServiceLabel } from './ServiceLabel';

afterEach(() => {
  cleanup();
});

describe('ServiceLabel', () => {
  test.each([
    { name: 'ロゴがあれば、名前の前に出す', logoUrl: '/media/services/1/logo', wantLogo: true },
    { name: 'ロゴがなければ、代わりの枠と名前を出す', logoUrl: null, wantLogo: false },
  ])('$name', ({ logoUrl, wantLogo }) => {
    const { container } = render(<ServiceLabel name="こたつテスト1" logoUrl={logoUrl} />);

    expect(screen.getByText('こたつテスト1')).toBeDefined();
    expect(container.querySelector('img')?.getAttribute('src') ?? null).toBe(
      wantLogo ? logoUrl : null,
    );
    expect(container.querySelector('[data-logo-placeholder]') !== null).toBe(!wantLogo);
  });

  test('ロゴを読めなければ、代わりの枠にし、URL が変われば読み直す', () => {
    const { container, rerender } = render(
      <ServiceLabel name="こたつテスト1" logoUrl="/media/services/1/logo" />,
    );

    fireEvent.error(container.querySelector('img')!);
    expect(container.querySelector('img')).toBeNull();
    expect(container.querySelector('[data-logo-placeholder]')).not.toBeNull();
    expect(screen.getByText('こたつテスト1')).toBeDefined();

    rerender(<ServiceLabel name="こたつテスト2" logoUrl="/media/services/2/logo" />);
    expect(container.querySelector('img')?.getAttribute('src')).toBe('/media/services/2/logo');
  });
});
