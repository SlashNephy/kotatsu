import { useEffect, useRef } from 'react';

import { runCommand, type PlayerCommands } from './commands';
import { shortcutFor, useKeymap } from './shortcuts';
import { useWatchMini } from './WatchHost';

// usePlayerShortcuts は、キーボードで、プレイヤーを操作できるようにする (設計書 7.5)。今使えない操作のキーは、何もしない。
// キーは、この端末の割り当て (#810) で引く。小さなプレイヤー (#1602) で描いている間は、ほかの画面の操作に譲り、何もしない。
export function usePlayerShortcuts(commands: PlayerCommands): void {
  const commandsRef = useRef(commands);
  commandsRef.current = commands;
  const miniRef = useRef(false);
  miniRef.current = useWatchMini();
  const [keymap] = useKeymap();
  const keymapRef = useRef(keymap);
  keymapRef.current = keymap;

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      const id = shortcutFor(event, keymapRef.current);
      if (miniRef.current || id === null || commandsRef.current[id] === undefined) {
        return;
      }
      // スペースでページが流れないようにする。
      event.preventDefault();
      runCommand(commandsRef.current, id);
    };
    window.addEventListener('keydown', onKeyDown);

    return () => window.removeEventListener('keydown', onKeyDown);
  }, []);
}
