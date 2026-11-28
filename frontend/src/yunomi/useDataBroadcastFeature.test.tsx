import { act, renderHook } from '@testing-library/react';
import { describe, expect, test, vi } from 'vitest';

import { RemoteKey } from './useDataBroadcast';
import {
  useDataBroadcastFeature,
  type DataBroadcastFeatureOptions,
} from './useDataBroadcastFeature';

// データ放送の文書を描く部品は、ブラウザの Worker と WebAssembly を使うので、テストでは押したボタンだけを記録する。
const pressed = vi.hoisted(() => [] as number[]);
vi.mock('./useDataBroadcast', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./useDataBroadcast')>();

  return {
    ...actual,
    useDataBroadcast: () => ({
      layer: null,
      state: 'off',
      receiving: false,
      keyGroups: new Set(),
      press: (key: number) => pressed.push(key),
    }),
  };
});

function renderFeature(options: Partial<DataBroadcastFeatureOptions> = {}) {
  return renderHook(() =>
    useDataBroadcastFeature({
      source: { url: '/data-broadcast', serviceId: 1024 },
      receiving: true,
      mediaRef: { current: null },
      ipEnabled: false,
      handlers: {},
      showFlash: vi.fn(),
      ...options,
    }),
  );
}

describe('useDataBroadcastFeature', () => {
  test('出していない間は、d ボタンで出し、ほかのボタンは文書に渡さない', () => {
    pressed.length = 0;
    const { result } = renderFeature();

    act(() => result.current.commands.pressKey?.(RemoteKey.up));
    expect(result.current.enabled).toBe(false);
    act(() => result.current.commands.pressKey?.(RemoteKey.d));
    expect(result.current.enabled).toBe(true);
    act(() => result.current.commands.pressKey?.(RemoteKey.up));

    expect(pressed).toEqual([RemoteKey.up]);
  });

  test.each([
    {
      name: 'データ放送のある映像なら、出し入れの操作を出す',
      source: { url: '/data-broadcast', serviceId: 1024 },
      toggle: true,
    },
    { name: 'データ放送のない映像では、出し入れの操作を出さない', source: null, toggle: false },
  ])('$name', ({ source, toggle }) => {
    const { result } = renderFeature({ source });

    expect(result.current.commands.toggleDataBroadcast !== undefined).toBe(toggle);
  });
});
