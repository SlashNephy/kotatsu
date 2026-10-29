import { useEffect, useRef } from 'react';

import { runCommand, type PlayerCommands } from './commands';
import { shortcutFor } from './shortcuts';

// usePlayerShortcuts は、キーボードで、プレイヤーを操作できるようにする (設計書 7.5)。今使えない操作のキーは、何もしない。
export function usePlayerShortcuts(commands: PlayerCommands): void {
  const commandsRef = useRef(commands);
  commandsRef.current = commands;

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      const id = shortcutFor(event);
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
