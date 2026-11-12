import { MantineProvider } from '@mantine/core';
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { Captions } from 'lucide-react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import '../i18n';
import { CommandButton, holdDelayMs, SeekStepButtons } from './PlayerControls';

afterEach(() => {
  cleanup();
});

describe('CommandButton', () => {
  test('押すと、登録簿の操作を呼ぶ', () => {
    const toggleCaptions = vi.fn();
    render(
      <MantineProvider>
        <CommandButton
          commands={{ toggleCaptions }}
          command="toggleCaptions"
          label="字幕"
          icon={Captions}
        />
      </MantineProvider>,
    );

    fireEvent.click(screen.getByRole('button', { name: '字幕' }));
    expect(toggleCaptions).toHaveBeenCalledOnce();
  });

  test('今使えない操作のボタンは、出さない', () => {
    // 実況のない局のように、その画面にない操作。
    render(
      <MantineProvider>
        <CommandButton
          commands={{ toggleCaptions: () => {} }}
          command="toggleJikkyo"
          label="実況"
          icon={Captions}
        />
      </MantineProvider>,
    );

    expect(screen.queryByRole('button')).toBeNull();
  });
});

describe('SeekStepButtons', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    // jsdom の要素には、ポインターを捕まえる API がない。
    Element.prototype.setPointerCapture = () => undefined;
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  const renderButtons = (onFastForward?: (held: boolean) => void) => {
    const commands = { seekBackward15: vi.fn(), seekForward15: vi.fn() };
    render(
      <MantineProvider>
        <SeekStepButtons
          commands={commands}
          {...(onFastForward === undefined ? {} : { onFastForward })}
        />
      </MantineProvider>,
    );

    return commands;
  };
  const forwardButton = () => screen.getByRole('button', { name: /15 秒進む/ });
  // press は、ポインターで押して、ms だけ待ってから離す。ブラウザと同じく、離したらクリックも届く。
  const press = (button: HTMLElement, ms: number) => {
    fireEvent.pointerDown(button, { button: 0, pointerId: 1 });
    act(() => {
      vi.advanceTimersByTime(ms);
    });
    fireEvent.pointerUp(button, { button: 0, pointerId: 1 });
    fireEvent.click(button);
  };

  test('押すと、15 秒戻るか進む', () => {
    const commands = renderButtons(() => {});

    fireEvent.click(screen.getByRole('button', { name: '15 秒戻る' }));
    press(forwardButton(), 100);

    expect(commands.seekBackward15).toHaveBeenCalledOnce();
    expect(commands.seekForward15).toHaveBeenCalledOnce();
  });

  test('進むボタンを押し続けると、離すまで早回しにし、離しても 15 秒進まない', () => {
    const onFastForward = vi.fn();
    const commands = renderButtons(onFastForward);

    fireEvent.pointerDown(forwardButton(), { button: 0, pointerId: 1 });
    act(() => {
      vi.advanceTimersByTime(holdDelayMs);
    });
    expect(onFastForward).toHaveBeenLastCalledWith(true);

    fireEvent.pointerUp(forwardButton(), { button: 0, pointerId: 1 });
    fireEvent.click(forwardButton());
    expect(onFastForward).toHaveBeenLastCalledWith(false);
    expect(commands.seekForward15).not.toHaveBeenCalled();

    // 押し続けた後でも、次に押したら、また 15 秒進む。
    act(() => {
      vi.runAllTimers();
    });
    fireEvent.click(forwardButton());
    expect(commands.seekForward15).toHaveBeenCalledOnce();
  });

  test('押し続けている間にウィンドウを離れたら、早回しをやめる', () => {
    const onFastForward = vi.fn();
    renderButtons(onFastForward);

    fireEvent.pointerDown(forwardButton(), { button: 0, pointerId: 1 });
    act(() => {
      vi.advanceTimersByTime(holdDelayMs);
    });
    fireEvent.blur(window);

    expect(onFastForward).toHaveBeenLastCalledWith(false);
  });

  test('押し続けている間にタブを隠したら、早回しをやめる', () => {
    const onFastForward = vi.fn();
    renderButtons(onFastForward);

    fireEvent.pointerDown(forwardButton(), { button: 0, pointerId: 1 });
    act(() => {
      vi.advanceTimersByTime(holdDelayMs);
    });
    const visibility = vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden');
    try {
      fireEvent(document, new Event('visibilitychange'));
    } finally {
      visibility.mockRestore();
    }

    expect(onFastForward).toHaveBeenLastCalledWith(false);
  });

  test('キーボードで押したら、早回しにせず 15 秒進む', () => {
    const onFastForward = vi.fn();
    const commands = renderButtons(onFastForward);

    // キーボードの Enter や Space では、ポインターのイベントは届かず、クリックだけが届く。
    fireEvent.click(forwardButton());
    act(() => {
      vi.advanceTimersByTime(holdDelayMs * 2);
    });

    expect(commands.seekForward15).toHaveBeenCalledOnce();
    expect(onFastForward).not.toHaveBeenCalled();
  });

  test('指がずれて取り消されたら、早回しをやめる', () => {
    const onFastForward = vi.fn();
    renderButtons(onFastForward);

    fireEvent.pointerDown(forwardButton(), { button: 0, pointerId: 1 });
    act(() => {
      vi.advanceTimersByTime(holdDelayMs);
    });
    fireEvent.pointerCancel(forwardButton(), { pointerId: 1 });

    expect(onFastForward).toHaveBeenLastCalledWith(false);
  });

  test('早回しを渡さなければ、押し続けても 15 秒進む', () => {
    const commands = renderButtons();

    press(forwardButton(), holdDelayMs * 2);

    expect(commands.seekForward15).toHaveBeenCalledOnce();
  });

  test('シークできない画面では、出さない', () => {
    render(
      <MantineProvider>
        <SeekStepButtons commands={{ togglePlay: () => {} }} onFastForward={() => {}} />
      </MantineProvider>,
    );

    expect(screen.queryByRole('button')).toBeNull();
  });
});
