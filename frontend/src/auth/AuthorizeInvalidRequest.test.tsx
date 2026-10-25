import { MantineProvider } from '@mantine/core';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';

import '../i18n';
import { AuthorizeInvalidRequest } from './AuthorizeInvalidRequest';

const errorRedirectUrl = 'https://evil.example/fake-login?error=unsupported_response_type&state=s';

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function renderView(url: string | null) {
  const assign = vi.fn();
  // jsdom の location.assign は差し替えられないので、assign だけを持つ location に替える。
  vi.stubGlobal('location', { assign });
  render(
    <MantineProvider>
      <AuthorizeInvalidRequest errorRedirectUrl={url} />
    </MantineProvider>,
  );

  return assign;
}

describe('AuthorizeInvalidRequest (#1014)', () => {
  test('戻す先へは勝手に移らず、ホストを見せる', () => {
    const assign = renderView(errorRedirectUrl);

    expect(assign).not.toHaveBeenCalled();
    expect(screen.getByText(/戻る先: evil\.example。/)).toBeDefined();
  });

  test('押すと、エラーを付けた戻す先へ移る', () => {
    const assign = renderView(errorRedirectUrl);

    fireEvent.click(screen.getByRole('button', { name: 'アプリに戻る' }));

    expect(assign).toHaveBeenCalledWith(errorRedirectUrl);
  });

  test('戻す先がなければ、エラーだけを出す', () => {
    renderView(null);

    expect(screen.getByRole('alert')).toBeDefined();
    expect(screen.queryByRole('button', { name: 'アプリに戻る' })).toBeNull();
  });
});
