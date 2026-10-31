import {
  Badge,
  Button,
  Card,
  CloseButton,
  Group,
  Kbd,
  Stack,
  Table,
  Text,
  Title,
} from '@mantine/core';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { simpleCommandIds, type SimpleCommandId } from '../yunomi/commands';
import { assignKey, eventKey, keysFor, removeKey, useKeymap } from '../yunomi/shortcuts';
import { keyLabels, splitKey } from '../yunomi/ShortcutsHelp';

// ShortcutKeysCard は、プレイヤーのキーの割り当てを、この端末で変える設定 (#810、設計書 7.5)。
// 「キーを足す」を押してから押したキーを、その操作に足す。ほかの操作に割り当てていたキーは、そちらから外す。
export function ShortcutKeysCard() {
  const { t } = useTranslation();
  const [keymap, setKeymap] = useKeymap();
  // capturing は、キーを待っている操作。Escape か、ほかの所へフォーカスを移すとやめる。
  const [capturing, setCapturing] = useState<SimpleCommandId | null>(null);

  return (
    <Card withBorder>
      <Stack gap="sm">
        <Group gap={6}>
          <Title order={3} size="h5">
            {t('settings.shortcutKeys.title')}
          </Title>
          {/* アカウントの設定と並ぶので、この端末だけの設定であることを示す。 */}
          <Badge size="sm" variant="light" color="gray" tt="none">
            {t('settings.account.deviceOnly')}
          </Badge>
        </Group>
        <Text size="sm" c="dimmed">
          {t('settings.shortcutKeys.description')}
        </Text>
        <Table.ScrollContainer minWidth={480}>
          <Table verticalSpacing={4}>
            <Table.Tbody>
              {simpleCommandIds.map((id) => (
                <Table.Tr key={id}>
                  <Table.Td>{t(`player.commands.${id}`)}</Table.Td>
                  <Table.Td>
                    <Group gap={6}>
                      {keysFor(id, keymap).map((key) => (
                        <Group key={key} gap={2} wrap="nowrap">
                          {splitKey(key).map((part, i) => (
                            <Kbd key={i}>{keyLabels[part] ?? part}</Kbd>
                          ))}
                          <CloseButton
                            size="xs"
                            aria-label={t('settings.shortcutKeys.remove', { key })}
                            onClick={() => setKeymap(removeKey(keymap, id, key))}
                          />
                        </Group>
                      ))}
                      <Button
                        size="compact-xs"
                        variant={capturing === id ? 'filled' : 'subtle'}
                        onClick={() => setCapturing(capturing === id ? null : id)}
                        onBlur={() => setCapturing(null)}
                        onKeyDown={(event) => {
                          if (capturing !== id) {
                            return;
                          }
                          // Tab でフォーカスを移せるよう、Tab は横取りしない。
                          if (event.key === 'Tab') {
                            setCapturing(null);

                            return;
                          }
                          event.preventDefault();
                          event.stopPropagation();
                          if (event.key === 'Escape') {
                            setCapturing(null);

                            return;
                          }
                          const key = eventKey(event);
                          if (key !== null) {
                            setKeymap(assignKey(keymap, id, key));
                            setCapturing(null);
                          }
                        }}
                      >
                        {capturing === id
                          ? t('settings.shortcutKeys.waiting')
                          : t('settings.shortcutKeys.add')}
                      </Button>
                    </Group>
                  </Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        </Table.ScrollContainer>
        <Group>
          <Button size="xs" variant="default" onClick={() => setKeymap(null)}>
            {t('settings.shortcutKeys.reset')}
          </Button>
        </Group>
      </Stack>
    </Card>
  );
}
