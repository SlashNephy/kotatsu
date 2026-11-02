import { cleanup, render, screen } from '@testing-library/react';
import { useContext } from 'react';
import { afterEach, describe, expect, test, vi } from 'vitest';

import { ErrorBoundary, ErrorBoundaryAttemptContext } from './ErrorBoundary';

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

  test('失敗している間に resetKey が変わったら、子を描き直す。失敗していなければ、描き直さない', () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    let fail = true;
    let mounts = 0;
    function Child() {
      const attempt = useContext(ErrorBoundaryAttemptContext);
      if (fail) {
        throw new Error('broken');
      }

      return <p ref={() => void (mounts += 1)}>attempt {attempt}</p>;
    }
    const view = (resetKey: string) => (
      <ErrorBoundary resetKey={resetKey} fallback={() => <p>failed</p>}>
        <Child />
      </ErrorBoundary>
    );

    const { rerender } = render(view('/a'));
    expect(screen.getByText('failed')).toBeDefined();

    fail = false;
    rerender(view('/a'));
    expect(screen.getByText('failed')).toBeDefined();

    rerender(view('/b'));
    expect(screen.getByText('attempt 1')).toBeDefined();

    rerender(view('/c'));
    expect(screen.getByText('attempt 1')).toBeDefined();
    expect(mounts).toBe(1);
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
