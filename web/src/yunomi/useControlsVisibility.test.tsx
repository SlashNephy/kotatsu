import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import { controlsHideDelayMs } from './controlsVisibility';
import { controlsAttribute, useControlsVisibility } from './useControlsVisibility';

function Harness({ playing }: { playing: boolean }) {
  const { visible, containerProps } = useControlsVisibility({ playing });

  return (
    <div data-testid="container" {...containerProps}>
      <div data-testid="video" />
      <div data-testid="controls" data-visible={visible} {...{ [controlsAttribute]: '' }}>
        <button type="button">play</button>
      </div>
    </div>
  );
}

const visible = () => screen.getByTestId('controls').dataset.visible === 'true';
const advance = (ms: number) => {
  act(() => {
    vi.advanceTimersByTime(ms);
  });
};

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe('useControlsVisibility', () => {
  test('止めている間は、時間が経っても隠さない', () => {
    render(<Harness playing={false} />);
    advance(controlsHideDelayMs * 2);

    expect(visible()).toBe(true);
  });

  test('再生中に操作がなくなると隠し、マウスが動くと出して数え直す', () => {
    render(<Harness playing />);
    advance(controlsHideDelayMs + 100);
    expect(visible()).toBe(false);

    fireEvent.pointerMove(screen.getByTestId('container'), { pointerType: 'mouse' });
    expect(visible()).toBe(true);
    advance(controlsHideDelayMs - 100);
    fireEvent.pointerMove(screen.getByTestId('container'), { pointerType: 'mouse' });
    advance(controlsHideDelayMs - 100);
    expect(visible()).toBe(true);
  });

  test('マウスがプレイヤーの外へ出たら、待たずに隠す', () => {
    render(<Harness playing />);
    fireEvent.pointerLeave(screen.getByTestId('container'), { pointerType: 'mouse' });

    expect(visible()).toBe(false);
  });

  test('映像のタップで、出すと隠すを切り替える', () => {
    render(<Harness playing />);
    const video = screen.getByTestId('video');

    fireEvent.pointerDown(video, { pointerType: 'touch' });
    expect(visible()).toBe(false);
    fireEvent.pointerDown(video, { pointerType: 'touch' });
    expect(visible()).toBe(true);
  });

  test('操作バーの中のタップでは隠さない', () => {
    render(<Harness playing />);
    fireEvent.pointerDown(screen.getByRole('button', { name: 'play' }), { pointerType: 'touch' });

    expect(visible()).toBe(true);
  });

  test('操作バーにフォーカスがある間は隠さない', () => {
    render(<Harness playing />);
    act(() => screen.getByRole('button', { name: 'play' }).focus());
    advance(controlsHideDelayMs * 2);

    expect(visible()).toBe(true);
  });
});
