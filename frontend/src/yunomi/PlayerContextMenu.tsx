import { Kbd, Menu, ScrollArea } from '@mantine/core';
import { useState, type MouseEvent, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

import {
  runCommand,
  simpleCommandIds,
  type PlayerCommands,
  type SimpleCommandId,
} from './commands';
import { keysFor, useKeymap } from './shortcuts';
import { keyLabels, splitKey } from './ShortcutsHelp';
import { onVideoSurface } from './useControlsVisibility';

// プレイヤーを右クリックしたときのメニュー (#1140、設計書 7.5)。ブラウザの動画用のメニューの代わりに、
// その画面で今使える操作を、操作の登録簿の順に並べる。

// hiddenCommands は、メニューに並べない操作。再生と一時停止は、切り替えがあるので並べない。
const hiddenCommands: ReadonlySet<SimpleCommandId> = new Set(['play', 'pause']);

// contextMenuItems は、メニューに並べる操作を返す。今使える、引数のない操作を、登録簿の順に並べる。
export function contextMenuItems(commands: PlayerCommands): SimpleCommandId[] {
  return simpleCommandIds.filter((id) => commands[id] !== undefined && !hiddenCommands.has(id));
}

// menuMaxHeight は、メニューの高さの上限。小さいプレイヤーでも収まるよう、中をスクロールさせる。
const menuMaxHeight = 360;

// usePlayerContextMenu は、プレイヤーを包む要素に渡す onContextMenu と、その中に描くメニューを返す。
// 映像の上の右クリック (Android では長押し) で、カーソルの位置にメニューを出す。操作バーの上では、ブラウザのメニューのままにする。
// Firefox の Shift+右クリックは、ブラウザがメニューを出すので、止めない。
export function usePlayerContextMenu(commands: PlayerCommands): {
  onContextMenu: (event: MouseEvent<HTMLElement>) => void;
  menu: ReactNode;
} {
  const { t } = useTranslation();
  const [keymap] = useKeymap();
  // position は、メニューを出す位置 (画面の座標)。閉じていれば null。
  const [position, setPosition] = useState<{ x: number; y: number } | null>(null);

  const onContextMenu = (event: MouseEvent<HTMLElement>) => {
    if (!onVideoSurface(event.target)) {
      return;
    }
    event.preventDefault();
    setPosition({ x: event.clientX, y: event.clientY });
  };

  const items = contextMenuItems(commands);
  const menu = (
    // メニューの中のクリックが、映像のクリック (再生と一時停止) に伝わらないよう止める。
    <div style={{ display: 'contents' }} onClick={(event) => event.stopPropagation()}>
      <Menu
        opened={position !== null}
        onChange={(opened) => {
          if (!opened) {
            setPosition(null);
          }
        }}
        position="bottom-start"
        offset={0}
        // 全画面と別のウィンドウでも見えるよう、プレイヤーの中に描く。
        withinPortal={false}
        shadow="md"
      >
        <Menu.Target>
          <div
            aria-hidden
            style={{
              position: 'fixed',
              left: position?.x ?? 0,
              top: position?.y ?? 0,
              width: 1,
              height: 1,
              pointerEvents: 'none',
            }}
          />
        </Menu.Target>
        <Menu.Dropdown>
          <ScrollArea.Autosize mah={menuMaxHeight} type="auto">
            {items.map((id) => {
              const key = keysFor(id, keymap)[0];

              return (
                <Menu.Item
                  key={id}
                  onClick={() => runCommand(commands, id)}
                  rightSection={
                    key === undefined ? undefined : (
                      <Kbd size="xs">
                        {splitKey(key)
                          .map((part) => keyLabels[part] ?? part)
                          .join('+')}
                      </Kbd>
                    )
                  }
                >
                  {t(`player.commands.${id}`)}
                </Menu.Item>
              );
            })}
          </ScrollArea.Autosize>
        </Menu.Dropdown>
      </Menu>
    </div>
  );

  return { onContextMenu, menu };
}
