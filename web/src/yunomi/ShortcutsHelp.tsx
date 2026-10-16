import { Group, Kbd, Modal, Table } from '@mantine/core';
import type { RefObject } from 'react';
import { useTranslation } from 'react-i18next';

import { shortcutKeys, type ShortcutAction } from './shortcuts';

// keyLabels は、記号で見せるキーの名前。ほかのキーは、そのまま見せる。
const keyLabels: Readonly<Record<string, string>> = {
  ' ': 'Space',
  ArrowLeft: '←',
  ArrowRight: '→',
};

type Props = {
  opened: boolean;
  onClose: () => void;
  // actions は、この画面で使える操作。並べる順も、これに従う。
  actions: readonly ShortcutAction[];
  // target は、一覧を描く要素。全画面や別のウィンドウでも見えるよう、プレイヤーの中に描く。
  target: RefObject<HTMLElement | null>;
};

// ShortcutsHelp は、キーボードショートカットの一覧 (設計書 7.5)。割り当ては shortcuts.ts から作る。
export function ShortcutsHelp({ opened, onClose, actions, target }: Props) {
  const { t } = useTranslation();

  return (
    <Modal
      opened={opened}
      onClose={onClose}
      title={t('player.shortcuts.title')}
      portalProps={{ target: target.current ?? undefined }}
    >
      <Table>
        <Table.Tbody>
          {actions.map((action) => (
            <Table.Tr key={action}>
              <Table.Td>{t(`player.shortcuts.${action}`)}</Table.Td>
              <Table.Td>
                <Group gap={4} wrap="nowrap">
                  {shortcutKeys[action].map((key) => (
                    <Kbd key={key}>{keyLabels[key] ?? key}</Kbd>
                  ))}
                </Group>
              </Table.Td>
            </Table.Tr>
          ))}
        </Table.Tbody>
      </Table>
    </Modal>
  );
}
