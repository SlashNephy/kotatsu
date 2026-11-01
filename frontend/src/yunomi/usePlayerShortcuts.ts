import { useEffect, useRef } from 'react';

import { runCommand, type PlayerCommands } from './commands';
import { shortcutFor, useKeymap } from './shortcuts';

// usePlayerShortcuts は、キーボードで、プレイヤーを操作できるようにする (設計書 7.5)。今使えない操作のキーは、何もしない。
// キーは、この端末の割り当て (#810) で引く。
export function usePlayerShortcuts(commands: PlayerCommands): void {
  const commandsRef = useRef(commands);
  commandsRef.current = commands;
  const [keymap] = useKeymap();
  const keymapRef = useRef(keymap);
  keymapRef.current = keymap;

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      const id = shortcutFor(event, keymapRef.current);
      if (id === null || commandsRef.current[id] === undefined) {
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
