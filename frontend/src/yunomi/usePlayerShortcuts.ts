import { useEffect, useRef } from 'react';

import { shortcutFor, type ShortcutAction } from './shortcuts';

// usePlayerShortcuts は、キーボードで、プレイヤーを操作できるようにする (設計書 7.5)。割り当てのない操作は、何もしない。
export function usePlayerShortcuts(actions: Partial<Record<ShortcutAction, () => void>>): void {
  const actionsRef = useRef(actions);
  actionsRef.current = actions;

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      const name = shortcutFor(event);
      const action = name === null ? undefined : actionsRef.current[name];
      if (action === undefined) {
        return;
      }
      // スペースでページが流れないようにする。
      event.preventDefault();
      action();
    };
    window.addEventListener('keydown', onKeyDown);

    return () => window.removeEventListener('keydown', onKeyDown);
  }, []);
}
