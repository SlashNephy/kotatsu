import { act, cleanup, render, screen } from '@testing-library/react';
import { type ReactNode, useState } from 'react';
import { afterEach, describe, expect, test } from 'vitest';

import { useHostedWatch, WatchHost, WatchHostProvider } from './WatchHost';

afterEach(() => {
  cleanup();
});

// Counter は、押した回数を覚える。置き場で描き直しても状態が残るかを確かめる。
function Counter({ label }: { label: string }) {
  const [count, setCount] = useState(0);

  return (
    <button type="button" onClick={() => setCount(count + 1)}>
      {label}:{count}
    </button>
  );
}

// Route は、ルートの代わり。useHostedWatch に渡した要素を、ルートの位置に返す。
function Route({ label }: { label: string }) {
  return <div data-testid="route">{useHostedWatch(<Counter label={label} />)}</div>;
}

// App は、アプリの根元の代わり。ルートと置き場を並べる。
function App({ route }: { route: ReactNode }) {
  return (
    <WatchHostProvider>
      {route}
      <div data-testid="host">
        <WatchHost />
      </div>
    </WatchHostProvider>
  );
}

describe('useHostedWatch', () => {
  test('ルートの位置ではなく、置き場で描く', () => {
    render(<App route={<Route label="live" />} />);

    expect(screen.getByTestId('host').textContent).toBe('live:0');
    expect(screen.getByTestId('route').textContent).toBe('');
  });

  test('ルートが描き直しても、置き場の要素は作り直さず、状態を保つ', () => {
    const { rerender } = render(<App route={<Route label="live" />} />);
    act(() => {
      screen.getByRole('button').click();
    });
    expect(screen.getByRole('button').textContent).toBe('live:1');

    rerender(<App route={<Route label="other" />} />);
    expect(screen.getByRole('button').textContent).toBe('other:1');
  });

  test('ルートを離れたら、置き場から下ろす', () => {
    const { rerender } = render(<App route={<Route label="live" />} />);
    rerender(<App route={null} />);

    expect(screen.getByTestId('host').textContent).toBe('');
  });

  test('置き場がなければ、その場で描く', () => {
    render(<Route label="live" />);

    expect(screen.getByTestId('route').textContent).toBe('live:0');
  });
});
