import { cleanNotifications } from '@mantine/notifications';
import { act, cleanup, fireEvent, screen } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import { createTestEnvironment, renderWithRouter } from '../test/route';
import { DataBroadcastZipcodeInput } from './DataBroadcastZipcodeInput';

afterEach(() => {
  cleanup();
  window.localStorage.clear();
});

async function render() {
  await renderWithRouter(
    <DataBroadcastZipcodeInput />,
    createTestEnvironment(() => undefined),
  );

  return screen.findByRole('textbox', { name: /データ放送の郵便番号/ });
}

describe('データ放送の郵便番号 (#1740)', () => {
  test('ハイフンや全角の数字は、数字だけにして保存し、知らせる', async () => {
    const input = await render();
    fireEvent.change(input, { target: { value: '１００-０００１' } });

    await act(async () => fireEvent.blur(input));

    expect(input).toHaveProperty('value', '1000001');
    expect((await screen.findByRole('status')).textContent).toContain('保存しました');
  });

  test('保存してある番号と同じなら、書き方が違っても保存も知らせもしない', async () => {
    const input = await render();
    fireEvent.change(input, { target: { value: '1000001' } });
    await act(async () => fireEvent.blur(input));
    // 1 回目の知らせを消して、2 回目に知らせが出ないことを確かめる。
    act(() => cleanNotifications());

    fireEvent.change(input, { target: { value: '100-0001' } });
    await act(async () => fireEvent.blur(input));

    expect(input).toHaveProperty('value', '1000001');
    expect(screen.queryByRole('status')).toBeNull();
  });

  test('7 桁でなければ、理由を出して保存しない', async () => {
    const input = await render();
    fireEvent.change(input, { target: { value: '12345' } });

    await act(async () => fireEvent.blur(input));

    expect(screen.getByText('郵便番号は7桁の数字で入れてください')).toBeDefined();
    expect(screen.queryByRole('status')).toBeNull();
  });
});
