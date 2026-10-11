import { Button, Checkbox, Group, Menu } from '@mantine/core';
import { useState, type RefObject } from 'react';
import { useTranslation } from 'react-i18next';

import { captureFrame, copyBlob, overlayCanvases, saveBlob } from './capture';
import classes from './TsPlayer.module.css';
import type { useDocumentPictureInPicture } from './useDocumentPictureInPicture';

type Props = {
  frameRef: RefObject<HTMLElement | null>;
  videoRef: RefObject<HTMLVideoElement | null>;
  // name は、キャプチャのファイル名に使う、番組名やチャンネル名。
  name: string;
  pip: ReturnType<typeof useDocumentPictureInPicture>;
  // onBeforePip は、別のウィンドウに出す前と戻す前に呼ぶ。プレイヤーを作り直すので、録画では再生位置を覚えておく。
  onBeforePip?: () => void;
};

// PlayerTools は、キャプチャと、別のウィンドウに出す操作 (設計書 7.5)。
export function PlayerTools({ frameRef, videoRef, name, pip, onBeforePip }: Props) {
  const { t } = useTranslation();
  // overlays は、キャプチャに、字幕とコメントを写すかどうか。
  const [overlays, setOverlays] = useState(true);
  const [message, setMessage] = useState<string | null>(null);

  const capture = async (action: 'save' | 'copy') => {
    const video = videoRef.current;
    const frame = frameRef.current;
    if (video === null || frame === null) {
      return;
    }
    try {
      const blob = await captureFrame(video, overlays ? overlayCanvases(frame, video) : []);
      if (action === 'save') {
        const stamp = new Date().toISOString().replaceAll(':', '-').replace(/\..*$/, '');
        saveBlob(blob, `${name}-${stamp}.png`);
      } else {
        await copyBlob(blob);
        setMessage(t('player.copied'));
      }
    } catch {
      setMessage(t('player.captureError'));
    }
  };

  return (
    <Group gap="xs">
      <Menu position="bottom-end" closeOnItemClick>
        <Menu.Target>
          <Button size="compact-sm" variant="default">
            {t('player.capture')}
          </Button>
        </Menu.Target>
        <Menu.Dropdown>
          <Menu.Item onClick={() => void capture('save')}>{t('player.captureSave')}</Menu.Item>
          <Menu.Item onClick={() => void capture('copy')}>{t('player.captureCopy')}</Menu.Item>
          <Menu.Divider />
          <Menu.Label>
            <Checkbox
              size="xs"
              label={t('player.captureOverlays')}
              checked={overlays}
              onChange={(event) => setOverlays(event.currentTarget.checked)}
            />
          </Menu.Label>
        </Menu.Dropdown>
      </Menu>
      {pip.supported && (
        <Button
          size="compact-sm"
          variant="default"
          onClick={() => {
            onBeforePip?.();
            if (pip.container === null) {
              void pip.open(640, 360, classes.pip ?? '');
            } else {
              pip.close();
            }
          }}
        >
          {pip.container === null ? t('player.pip') : t('player.pipBack')}
        </Button>
      )}
      {message !== null && (
        <span role="status" className={classes.toolMessage}>
          {message}
        </span>
      )}
    </Group>
  );
}
