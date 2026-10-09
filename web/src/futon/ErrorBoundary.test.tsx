import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';

import { ErrorBoundary } from './ErrorBoundary';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function Throws(): never {
  throw new Error('broken');
}

describe('ErrorBoundary', () => {
  test('子の描画のエラーを捕まえ、そのエラーで fallback を出す', () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined);

    render(
      <ErrorBoundary fallback={(error) => <p>failed: {error.message}</p>}>
        <Throws />
      </ErrorBoundary>,
    );

    expect(screen.getByText('failed: broken')).toBeDefined();
  });

  test('エラーがなければ子を出す', () => {
    render(
      <ErrorBoundary fallback={() => <p>failed</p>}>
        <p>ok</p>
      </ErrorBoundary>,
    );

    expect(screen.getByText('ok')).toBeDefined();
    expect(screen.queryByText('failed')).toBeNull();
  });
});
