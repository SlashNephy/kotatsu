import { MantineProvider } from '@mantine/core';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterAll, afterEach, beforeAll, describe, expect, test, vi } from 'vitest';

import '../i18n';
import { SeekBar } from './SeekBar';

// jsdom は PointerEvent を持たないので、位置と種類を運べるよう、MouseEvent から作る。
class TestPointerEvent extends MouseEvent {
  readonly pointerType: string;

  constructor(type: string, init: PointerEventInit = {}) {
    super(type, init);
    this.pointerType = init.pointerType ?? 'mouse';
  }
}

beforeAll(() => {
  vi.stubGlobal('PointerEvent', TestPointerEvent);
});

afterAll(() => {
  vi.unstubAllGlobals();
});

afterEach(() => {
  cleanup();
});

// renderSeekBar は、幅 600px で左端が 0 の、長さ 600 秒のシークバーを描く。jsdom は大きさを持たないので、測った値を差し替える。
function renderSeekBar() {
  const { container } = render(
    <MantineProvider>
      <SeekBar
        duration={600}
        value={0}
        label={(seconds) => `${Math.round(seconds)} 秒`}
        previewUrl={null}
        onChange={() => {}}
        onChangeEnd={() => {}}
      />
    </MantineProvider>,
  );
  // MantineProvider は先頭に style の要素を置くので、最初の div をシークバーとする。
  const root = container.querySelector('div')!;
  root.getBoundingClientRect = () => new DOMRect(0, 0, 600, 16);

  return root;
}

describe('SeekBar', () => {
  test('カーソルを乗せた位置の時刻を出し、バーから出たら消す', () => {
    const root = renderSeekBar();

    fireEvent.pointerMove(root, { clientX: 150, pointerType: 'mouse' });
    expect(screen.getByText('150 秒')).toBeDefined();

    fireEvent.pointerLeave(root, { pointerType: 'mouse' });
    expect(screen.queryByText('150 秒')).toBeNull();
  });

  test('つまみを動かしている間は、カーソルがバーから出ても消さない', () => {
    const root = renderSeekBar();

    fireEvent.pointerDown(root, { clientX: 300, pointerType: 'mouse' });
    fireEvent.pointerLeave(root, { pointerType: 'mouse' });
    expect(screen.getByText('300 秒')).toBeDefined();
  });

  test('タッチでは、指を離したら消す', () => {
    const root = renderSeekBar();

    fireEvent.pointerMove(root, { clientX: 60, pointerType: 'touch' });
    expect(screen.getByText('60 秒')).toBeDefined();

    fireEvent.pointerCancel(root, { pointerType: 'touch' });
    expect(screen.queryByText('60 秒')).toBeNull();
  });
});
