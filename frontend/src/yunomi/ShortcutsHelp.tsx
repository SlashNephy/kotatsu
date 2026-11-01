import { Group, Kbd, Modal, Table, Text } from '@mantine/core';
import type { RefObject } from 'react';
import { useTranslation } from 'react-i18next';

import { playerCommandIds, type PlayerCommands } from './commands';
import { dataBroadcastKeyHelp } from './dataBroadcastKeys';
import { keysFor, useKeymap } from './shortcuts';

// keyLabels は、記号で見せるキーの名前。ほかのキーは、そのまま見せる。
export const keyLabels: Readonly<Record<string, string>> = {
  ' ': 'Space',
  ArrowLeft: '←',
  ArrowRight: '→',
  ArrowUp: '↑',
  ArrowDown: '↓',
};

// splitKey は、`Shift+ArrowLeft` を、Shift と ArrowLeft に分ける。`+` のキーそのものは分けない。
export function splitKey(key: string): string[] {
  return key.length > 1 && key.startsWith('Shift+') ? ['Shift', key.slice('Shift+'.length)] : [key];
}

type Props = {
  opened: boolean;
  onClose: () => void;
  // commands は、この画面で今使える操作。このうち、キーを割り当てた操作を、登録簿の順に並べる。
  commands: PlayerCommands;
  // target は、一覧を描く要素。全画面や別のウィンドウでも見えるよう、プレイヤーの中に描く。
  target: RefObject<HTMLElement | null>;
  // dataBroadcast は、データ放送を出している間に使えるキーも並べるかどうか。ライブ視聴だけで使う (#441)。
  dataBroadcast?: boolean;
};

// ShortcutsHelp は、キーボードショートカットの一覧 (設計書 7.5)。割り当ては shortcuts.ts から作る。
export function ShortcutsHelp({ opened, onClose, commands, target, dataBroadcast = false }: Props) {
  const { t } = useTranslation();
  const [keymap] = useKeymap();
  const rows = playerCommandIds
    .filter((id) => commands[id] !== undefined)
    .map((id) => ({ id, keys: keysFor(id, keymap) }))
    .filter(({ keys }) => keys.length > 0);

  // 一覧はプレイヤーの中に描くので、一覧のクリックが、映像のクリック (再生と一時停止) に伝わらないよう止める。
  // React のイベントは、ポータルの中からも、React の親へ伝わる。
  return (
    <div style={{ display: 'contents' }} onClick={(event) => event.stopPropagation()}>
      <Modal
        opened={opened}
        onClose={onClose}
        title={t('player.shortcuts.title')}
        portalProps={{ target: target.current ?? undefined }}
      >
        <Table>
          <Table.Tbody>
            {rows.map(({ id, keys }) => (
              <Table.Tr key={id}>
                <Table.Td>{t(`player.commands.${id}`)}</Table.Td>
                <Table.Td>
                  <Group gap={4} wrap="nowrap">
                    {keys.map((key) => (
                      // Shift+← などは、キーを 1 つずつ並べる。
                      <Group key={key} gap={2} wrap="nowrap">
                        {splitKey(key).map((part, i) => (
                          <Kbd key={i}>{keyLabels[part] ?? part}</Kbd>
                        ))}
                      </Group>
                    ))}
                  </Group>
                </Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
        {dataBroadcast && (
          <>
            <Text fw={600} size="sm" mt="md">
              {t('player.shortcuts.dataBroadcastTitle')}
            </Text>
            <Table>
              <Table.Tbody>
                {dataBroadcastKeyHelp.map(({ label, keys }) => (
                  <Table.Tr key={label}>
                    <Table.Td>{t(`player.shortcuts.dataBroadcast.${label}`)}</Table.Td>
                    <Table.Td>
                      <Group gap={4} wrap="nowrap">
                        {/* 範囲 (数字の 0 から 9 など) は、1 つのキーとして出す。 */}
                        {keys.map((key) => {
                          const text = typeof key === 'string' ? key : t('range', key);

                          return <Kbd key={text}>{text}</Kbd>;
                        })}
                      </Group>
                    </Table.Td>
                  </Table.Tr>
                ))}
              </Table.Tbody>
            </Table>
          </>
        )}
      </Modal>
    </div>
  );
}
