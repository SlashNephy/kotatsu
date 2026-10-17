import { renderHook } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';

import { dataBroadcastKeyFor, useDataBroadcastKeys } from './dataBroadcastKeys';
import { RemoteKey, type KeyGroup } from './useDataBroadcast';

type Init = Partial<{
  key: string;
  code: string;
  altKey: boolean;
  ctrlKey: boolean;
  metaKey: boolean;
  repeat: boolean;
  defaultPrevented: boolean;
  target: EventTarget | null;
}>;

function key(init: Init) {
  return {
    key: '',
    code: '',
    altKey: false,
    ctrlKey: false,
    metaKey: false,
    repeat: false,
    defaultPrevented: false,
    target: null,
    ...init,
  };
}

describe('dataBroadcastKeyFor', () => {
  test.each([
    { name: '⌥+↑ は上', event: key({ key: 'ArrowUp', altKey: true }), want: RemoteKey.up },
    {
      name: '⌥+→ を押し続けると繰り返す',
      event: key({ key: 'ArrowRight', altKey: true, repeat: true }),
      want: RemoteKey.right,
    },
    {
      name: '修飾キーのない ↑ はプレイヤーの音量に残す',
      event: key({ key: 'ArrowUp' }),
      want: null,
    },
    { name: 'Enter は決定', event: key({ key: 'Enter' }), want: RemoteKey.enter },
    { name: 'Backspace は戻る', event: key({ key: 'Backspace' }), want: RemoteKey.back },
    { name: '数字', event: key({ key: '7' }), want: RemoteKey.digit7 },
    { name: '数字は押し続けても繰り返さない', event: key({ key: '7', repeat: true }), want: null },
    {
      name: '⌥+r は赤 (macOS で key が変わっても code で見る)',
      event: key({ key: '®', code: 'KeyR', altKey: true }),
      want: RemoteKey.red,
    },
    {
      name: '⌥+b は青',
      event: key({ key: '∫', code: 'KeyB', altKey: true }),
      want: RemoteKey.blue,
    },
    {
      name: '修飾キーのない r はプレイヤーの読み直しに残す',
      event: key({ key: 'r', code: 'KeyR' }),
      want: null,
    },
    {
      name: 'Ctrl と一緒なら横取りしない',
      event: key({ key: 'Enter', ctrlKey: true }),
      want: null,
    },
    {
      name: 'データ放送の文書が受け取ったキーは押さない',
      event: key({ key: 'Enter', defaultPrevented: true }),
      want: null,
    },
    {
      name: '入力欄の中では横取りしない',
      event: key({ key: '1', target: document.createElement('input') }),
      want: null,
    },
  ])('$name', ({ event, want }) => {
    expect(dataBroadcastKeyFor(event)).toBe(want);
  });
});

describe('useDataBroadcastKeys', () => {
  const keyDown = (init: KeyboardEventInit) => {
    const event = new KeyboardEvent('keydown', { cancelable: true, ...init });
    window.dispatchEvent(event);

    return event;
  };
  let unmount: (() => void) | null = null;
  afterEach(() => {
    unmount?.();
    unmount = null;
  });

  test.each([
    {
      name: '文書が使うグループのキーは押し、既定の動きを止める',
      groups: ['basic'],
      init: { key: 'Enter' },
      want: RemoteKey.enter,
    },
    {
      name: '文書が使わないグループのキーは押さない',
      groups: ['basic', 'data-button'],
      init: { key: '1' },
      want: null,
    },
  ] as { name: string; groups: KeyGroup[]; init: KeyboardEventInit; want: number | null }[])(
    '$name',
    ({ groups, init, want }) => {
      const press = vi.fn();
      ({ unmount } = renderHook(() => useDataBroadcastKeys(true, new Set(groups), press)));

      const event = keyDown(init);

      expect(press.mock.calls).toEqual(want === null ? [] : [[want]]);
      expect(event.defaultPrevented).toBe(want !== null);
    },
  );

  test('操作できない間と、外した後は押さない', () => {
    const press = vi.fn();
    const { rerender, unmount: dispose } = renderHook(
      ({ enabled }) => useDataBroadcastKeys(enabled, new Set<KeyGroup>(['basic']), press),
      { initialProps: { enabled: false } },
    );
    keyDown({ key: 'Enter' });
    rerender({ enabled: true });
    keyDown({ key: 'Enter' });
    dispose();
    keyDown({ key: 'Enter' });

    expect(press).toHaveBeenCalledExactlyOnceWith(RemoteKey.enter);
  });
});
