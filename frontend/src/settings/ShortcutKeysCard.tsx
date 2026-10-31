import {
  Badge,
  Button,
  Card,
  CloseButton,
  Group,
  Kbd,
  Select,
  Stack,
  Table,
  Text,
  Title,
} from '@mantine/core';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { simpleCommandIds, type SimpleCommandId } from '../yunomi/commands';
import {
  assignButton,
  buttonLabel,
  removeButton,
  useGamepadButtons,
  useGamepadMap,
} from '../yunomi/gamepad';
import {
  defaultMouseActions,
  middleClickActions,
  useMouseActions,
  wheelActions,
} from '../yunomi/mouseActions';
import { assignKey, eventKey, keysFor, removeKey, useKeymap } from '../yunomi/shortcuts';
import { keyLabels, splitKey } from '../yunomi/ShortcutsHelp';

// ShortcutKeysCard は、プレイヤーのキーの割り当てを、この端末で変える設定 (#810、設計書 7.5)。
// 「キーを足す」を押してから押したキーを、その操作に足す。ほかの操作に割り当てていたキーは、そちらから外す。
export function ShortcutKeysCard() {
  const { t } = useTranslation();
  const [keymap, setKeymap] = useKeymap();
  // capturing は、キーを待っている操作。Escape か、ほかの所へフォーカスを移すとやめる。
  const [capturing, setCapturing] = useState<SimpleCommandId | null>(null);
  const [gamepadMap, setGamepadMap] = useGamepadMap();
  const [mouseActions, setMouseActions] = useMouseActions();
  // capturingButton は、ゲームパッドのボタンを待っている操作。
  const [capturingButton, setCapturingButton] = useState<SimpleCommandId | null>(null);
  // 同じフレームで 2 つ押されても、1 つだけを割り当てるよう、待っている操作は ref でも持つ。
  const capturingButtonRef = useRef(capturingButton);
  capturingButtonRef.current = capturingButton;
  useGamepadButtons((button) => {
    const id = capturingButtonRef.current;
    if (id !== null) {
      capturingButtonRef.current = null;
      setGamepadMap(assignButton(gamepadMap, id, button));
      setCapturingButton(null);
    }
  });
  // startCapture は、キーかボタンを待ち始める。同時には 1 つだけを待つ。
  const startCapture = (kind: 'key' | 'button', id: SimpleCommandId | null) => {
    setCapturing(kind === 'key' ? id : null);
    setCapturingButton(kind === 'button' ? id : null);
  };
  // キーを待っている間は、ページ全体のキーを先に受け取る。Safari と macOS の Firefox は、押したボタンにフォーカスを
  // 移さないので、ボタンのキーの処理では受け取れない。ほかの所を押したら、待つのをやめる。
  // ボタンを待っている間も、Esc かほかの所を押したらやめる。
  useEffect(() => {
    if (capturingButton === null) {
      return;
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setCapturingButton(null);
      }
    };
    const onPointerDown = (event: PointerEvent) => {
      if (!(event.target instanceof Element) || event.target.closest('[data-capturing]') === null) {
        setCapturingButton(null);
      }
    };
    window.addEventListener('keydown', onKeyDown, true);
    window.addEventListener('pointerdown', onPointerDown, true);

    return () => {
      window.removeEventListener('keydown', onKeyDown, true);
      window.removeEventListener('pointerdown', onPointerDown, true);
    };
  }, [capturingButton]);
  useEffect(() => {
    if (capturing === null) {
      return;
    }
    const onKeyDown = (event: KeyboardEvent) => {
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
        setKeymap(assignKey(keymap, capturing, key));
        setCapturing(null);
      }
    };
    const onPointerDown = (event: PointerEvent) => {
      if (!(event.target instanceof Element) || event.target.closest('[data-capturing]') === null) {
        setCapturing(null);
      }
    };
    window.addEventListener('keydown', onKeyDown, true);
    window.addEventListener('pointerdown', onPointerDown, true);

    return () => {
      window.removeEventListener('keydown', onKeyDown, true);
      window.removeEventListener('pointerdown', onPointerDown, true);
    };
  }, [capturing, keymap, setKeymap]);

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
                            aria-label={t('settings.shortcutKeys.remove', {
                              key: keyLabels[key] ?? key,
                            })}
                            onClick={() => setKeymap(removeKey(keymap, id, key))}
                          />
                        </Group>
                      ))}
                      <Button
                        size="compact-xs"
                        variant={capturing === id ? 'filled' : 'subtle'}
                        onClick={() => startCapture('key', capturing === id ? null : id)}
                        data-capturing={capturing === id ? '' : undefined}
                      >
                        {capturing === id
                          ? t('settings.shortcutKeys.waiting')
                          : t('settings.shortcutKeys.add')}
                      </Button>
                    </Group>
                    <Group gap={6} mt={4}>
                      {(gamepadMap[id] ?? []).map((button) => (
                        <Group key={button} gap={2} wrap="nowrap">
                          <Kbd>{buttonLabel(button)}</Kbd>
                          <CloseButton
                            size="xs"
                            aria-label={t('settings.shortcutKeys.remove', {
                              key: buttonLabel(button),
                            })}
                            onClick={() => setGamepadMap(removeButton(gamepadMap, id, button))}
                          />
                        </Group>
                      ))}
                      <Button
                        size="compact-xs"
                        variant={capturingButton === id ? 'filled' : 'subtle'}
                        onClick={() => startCapture('button', capturingButton === id ? null : id)}
                        data-capturing={capturingButton === id ? '' : undefined}
                      >
                        {capturingButton === id
                          ? t('settings.shortcutKeys.waitingButton')
                          : t('settings.shortcutKeys.addButton')}
                      </Button>
                    </Group>
                  </Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        </Table.ScrollContainer>
        {/* マウスの割り当て。映像の上のホイールと中クリックで何をするか (#810)。 */}
        <Group gap="md" align="flex-end">
          <Select
            size="xs"
            label={t('settings.shortcutKeys.wheel')}
            value={mouseActions.wheel}
            allowDeselect={false}
            data={wheelActions.map((action) => ({
              value: action,
              label: t(`settings.shortcutKeys.wheelActions.${action}`),
            }))}
            onChange={(value) => {
              const wheel = wheelActions.find((action) => action === value);
              if (wheel !== undefined) {
                setMouseActions({ ...mouseActions, wheel });
              }
            }}
            maw={240}
          />
          <Select
            size="xs"
            label={t('settings.shortcutKeys.middleClick')}
            value={mouseActions.middleClick}
            allowDeselect={false}
            data={middleClickActions.map((action) => ({
              value: action,
              label:
                action === 'none'
                  ? t('settings.shortcutKeys.none')
                  : t(`player.commands.${action}`),
            }))}
            onChange={(value) => {
              const middleClick = middleClickActions.find((action) => action === value);
              if (middleClick !== undefined) {
                setMouseActions({ ...mouseActions, middleClick });
              }
            }}
            maw={240}
          />
        </Group>
        <Group>
          <Button
            size="xs"
            variant="default"
            onClick={() => {
              setKeymap(null);
              setGamepadMap(null);
              setMouseActions(defaultMouseActions);
            }}
          >
            {t('settings.shortcutKeys.reset')}
          </Button>
        </Group>
      </Stack>
    </Card>
  );
}
