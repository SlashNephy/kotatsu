import { ActionIcon, Button, Group, Text } from '@mantine/core';
import { useTranslation } from 'react-i18next';

import type { DataBroadcastState, RemoteKey } from './useDataBroadcast';
import { RemoteKey as Keys } from './useDataBroadcast';

// colorKeys は、リモコンの色のボタン。並びは、受信機のリモコンと同じ青、赤、緑、黄にする。
const colorKeys = [
  { key: Keys.blue, color: 'blue', label: 'player.remote.blue' },
  { key: Keys.red, color: 'red', label: 'player.remote.red' },
  { key: Keys.green, color: 'green', label: 'player.remote.green' },
  { key: Keys.yellow, color: 'yellow', label: 'player.remote.yellow' },
] as const;

// DataBroadcastRemote は、データ放送を操作するリモコンのボタン (設計書 7.4)。
export function DataBroadcastRemote({
  state,
  onPress,
}: {
  state: DataBroadcastState;
  onPress: (key: RemoteKey) => void;
}) {
  const { t } = useTranslation();
  const disabled = state !== 'shown';

  return (
    <Group gap="xs" wrap="wrap" role="group" aria-label={t('player.remote.title')}>
      <Group gap={4}>
        <Button
          size="compact-sm"
          variant="default"
          disabled={disabled}
          aria-label={t('player.remote.left')}
          onClick={() => onPress(Keys.left)}
        >
          ←
        </Button>
        <Button
          size="compact-sm"
          variant="default"
          disabled={disabled}
          aria-label={t('player.remote.up')}
          onClick={() => onPress(Keys.up)}
        >
          ↑
        </Button>
        <Button
          size="compact-sm"
          variant="default"
          disabled={disabled}
          aria-label={t('player.remote.down')}
          onClick={() => onPress(Keys.down)}
        >
          ↓
        </Button>
        <Button
          size="compact-sm"
          variant="default"
          disabled={disabled}
          aria-label={t('player.remote.right')}
          onClick={() => onPress(Keys.right)}
        >
          →
        </Button>
      </Group>
      <Button
        size="compact-sm"
        variant="default"
        disabled={disabled}
        onClick={() => onPress(Keys.enter)}
      >
        {t('player.remote.enter')}
      </Button>
      <Button
        size="compact-sm"
        variant="default"
        disabled={disabled}
        onClick={() => onPress(Keys.back)}
      >
        {t('player.remote.back')}
      </Button>
      <Group gap={4}>
        {colorKeys.map(({ key, color, label }) => (
          <ActionIcon
            key={key}
            size="sm"
            variant="filled"
            color={color}
            disabled={disabled}
            aria-label={t(label)}
            onClick={() => onPress(key)}
          />
        ))}
      </Group>
      {state === 'loading' && (
        <Text size="xs" c="dimmed">
          {t('player.dataBroadcastLoading')}
        </Text>
      )}
      {state === 'ended' && (
        <Text size="xs" c="dimmed">
          {t('player.dataBroadcastEnded')}
        </Text>
      )}
    </Group>
  );
}
