import { ActionIcon, Button, Group, SimpleGrid, Text } from '@mantine/core';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { playerControlsClasses } from './PlayerControls';
import { controlsAttribute } from './useControlsVisibility';
import type { DataBroadcastState, KeyGroup, RemoteKey } from './useDataBroadcast';
import { keyGroup, RemoteKey as Keys } from './useDataBroadcast';

// colorKeys は、リモコンの色のボタン。並びは、受信機のリモコンと同じ青、赤、緑、黄にする。
const colorKeys = [
  { key: Keys.blue, color: 'blue', label: 'player.remote.blue' },
  { key: Keys.red, color: 'red', label: 'player.remote.red' },
  { key: Keys.green, color: 'green', label: 'player.remote.green' },
  { key: Keys.yellow, color: 'yellow', label: 'player.remote.yellow' },
] as const;

// digitKeys は、リモコンの数字のボタン。受信機のリモコンと同じく 1〜12 を並べ、最後に 0 を置く。
const digitKeys = [
  { key: Keys.digit1, label: '1' },
  { key: Keys.digit2, label: '2' },
  { key: Keys.digit3, label: '3' },
  { key: Keys.digit4, label: '4' },
  { key: Keys.digit5, label: '5' },
  { key: Keys.digit6, label: '6' },
  { key: Keys.digit7, label: '7' },
  { key: Keys.digit8, label: '8' },
  { key: Keys.digit9, label: '9' },
  { key: Keys.digit10, label: '10' },
  { key: Keys.digit11, label: '11' },
  { key: Keys.digit12, label: '12' },
  { key: Keys.digit0, label: '0' },
] as const;

// DataBroadcastRemote は、データ放送を操作するリモコンのボタン (設計書 7.4)。
export function DataBroadcastRemote({
  state,
  receiving,
  keyGroups,
  onPress,
}: {
  state: DataBroadcastState;
  // receiving は、データ放送の画面を出せるまで、受け取っているか、d ボタンで入っている途中であること。
  receiving: boolean;
  // keyGroups は、いまの文書が使うキーのグループ。使わないグループのボタンは押せなくする (#441)。
  keyGroups: ReadonlySet<KeyGroup>;
  onPress: (key: RemoteKey) => void;
}) {
  const { t } = useTranslation();
  // 矢印は、押し続けると繰り返す (#441)。
  const repeat = useRepeatPress(onPress);
  // digitsOpen は、数字のボタンを開いているか。使う文書は少なく、場所を取るので、既定では閉じる。
  const [digitsOpen, setDigitsOpen] = useState(false);
  // データ放送の画面が出て、操作を受けられるようになるまでは押せない。
  const ready = state === 'shown' && !receiving;
  const disabledFor = (group: KeyGroup) => !ready || !keyGroups.has(group);
  const disabled = disabledFor(keyGroup(Keys.enter));
  const colorDisabled = disabledFor(keyGroup(Keys.blue));
  const digitsDisabled = disabledFor(keyGroup(Keys.digit0));

  return (
    <Group gap="xs" wrap="wrap" role="group" aria-label={t('player.remote.title')}>
      <Group gap={4}>
        <Button
          size="compact-sm"
          variant="default"
          disabled={disabled}
          aria-label={t('player.remote.left')}
          {...repeat.handlers(Keys.left)}
        >
          ←
        </Button>
        <Button
          size="compact-sm"
          variant="default"
          disabled={disabled}
          aria-label={t('player.remote.up')}
          {...repeat.handlers(Keys.up)}
        >
          ↑
        </Button>
        <Button
          size="compact-sm"
          variant="default"
          disabled={disabled}
          aria-label={t('player.remote.down')}
          {...repeat.handlers(Keys.down)}
        >
          ↓
        </Button>
        <Button
          size="compact-sm"
          variant="default"
          disabled={disabled}
          aria-label={t('player.remote.right')}
          {...repeat.handlers(Keys.right)}
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
      {/* d ボタンは、文書に渡す (#433)。データ放送の出し入れは、操作バーのボタンと d キーで行う。 */}
      <Button
        size="compact-sm"
        variant="default"
        disabled={disabled}
        aria-label={t('player.remote.dLabel')}
        onClick={() => onPress(Keys.d)}
      >
        d
      </Button>
      <Group gap={4}>
        {colorKeys.map(({ key, color, label }) => (
          <ActionIcon
            key={key}
            size="sm"
            variant="filled"
            color={color}
            disabled={colorDisabled}
            aria-label={t(label)}
            onClick={() => onPress(key)}
          />
        ))}
      </Group>
      <Button
        size="compact-sm"
        variant={digitsOpen ? 'light' : 'default'}
        aria-expanded={digitsOpen}
        onClick={() => setDigitsOpen(!digitsOpen)}
      >
        {t('player.remote.digits')}
      </Button>
      {digitsOpen && (
        <SimpleGrid cols={{ base: 7, sm: 13 }} spacing={4} verticalSpacing={4} maw={560}>
          {digitKeys.map(({ key, label }) => (
            <Button
              key={key}
              size="compact-sm"
              variant="default"
              disabled={digitsDisabled}
              aria-label={t('player.remote.digit', { digit: label })}
              onClick={() => onPress(key)}
            >
              {label}
            </Button>
          ))}
        </SimpleGrid>
      )}
      {state === 'ended' && (
        <Text size="xs" c="dimmed">
          {t('player.dataBroadcastEnded')}
        </Text>
      )}
    </Group>
  );
}

// DataBroadcastRemoteOverlay は、全画面と別のウィンドウで、映像の左下に重ねるデータ放送のリモコン (#435)。
// 映像を隠さないよう、既定では閉じ、開くボタンだけを出す。
export function DataBroadcastRemoteOverlay(props: Parameters<typeof DataBroadcastRemote>[0]) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);

  return (
    <div className={playerControlsClasses.remote} {...{ [controlsAttribute]: '' }}>
      {open && (
        <div className={playerControlsClasses.remotePanel}>
          <DataBroadcastRemote {...props} />
        </div>
      )}
      <Button
        size="compact-sm"
        variant={open ? 'white' : 'default'}
        color="dark"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
      >
        {t(open ? 'player.remote.close' : 'player.remote.open')}
      </Button>
    </div>
  );
}

// repeatDelayMs と repeatIntervalMs は、矢印を押し続けたときに、繰り返し始めるまでの時間と、繰り返す間隔。
export const repeatDelayMs = 400;
export const repeatIntervalMs = 120;

// useRepeatPress は、押し続けると繰り返すボタンのハンドラーを返す。ポインターで押したときに 1 回押し、押し続けると繰り返す。
// キーボードでボタンを押したときは click だけが届くので、そのときに 1 回押す。キーボードの click は detail が 0 になる。
// ポインターの後に届く click (detail が 1 以上) では押さない。タッチでは pointerleave が click より先に届くので、印では見分けない。
function useRepeatPress(onPress: (key: RemoteKey) => void) {
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const stop = () => {
    if (timer.current !== null) {
      clearTimeout(timer.current);
      timer.current = null;
    }
  };
  useEffect(() => stop, []);

  return {
    handlers: (key: RemoteKey) => ({
      onPointerDown: (event: { button: number }) => {
        if (event.button !== 0) {
          return;
        }
        stop();
        onPress(key);
        const again = (delay: number) => {
          timer.current = setTimeout(() => {
            onPress(key);
            again(repeatIntervalMs);
          }, delay);
        };
        again(repeatDelayMs);
      },
      onPointerUp: stop,
      onPointerLeave: stop,
      onPointerCancel: stop,
      onClick: (event: { detail: number }) => {
        if (event.detail === 0) {
          onPress(key);
        }
      },
    }),
  };
}
