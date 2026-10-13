import { ActionIcon, Checkbox, Menu, Tooltip } from '@mantine/core';
import { Camera, PictureInPicture2 } from 'lucide-react';
import { useEffect, useState, type RefObject } from 'react';
import { useTranslation } from 'react-i18next';

import { captureFrame, copyBlob, overlayCanvases, saveBlob } from './capture';
import { ControlButton, playerControlsClasses } from './PlayerControls';
import classes from './TsPlayer.module.css';
import type { useDocumentPictureInPicture } from './useDocumentPictureInPicture';

// useNotice は、少しの間だけ出す知らせ。
function useNotice(): [string | null, (message: string) => void] {
  const [message, setMessage] = useState<string | null>(null);
  useEffect(() => {
    if (message === null) {
      return;
    }
    const timer = setTimeout(() => setMessage(null), 3000);

    return () => clearTimeout(timer);
  }, [message]);

  return [message, setMessage];
}

type CaptureProps = {
  frameRef: RefObject<HTMLElement | null>;
  videoRef: RefObject<HTMLVideoElement | null>;
  // name は、キャプチャのファイル名に使う、番組名やチャンネル名。
  name: string;
};

// CaptureButton は、今の場面を画像に保存するか、コピーするメニュー (設計書 7.5)。
// 別のウィンドウに移っても開けるよう、メニューと知らせはプレイヤーの中に描く。
export function CaptureButton({ frameRef, videoRef, name }: CaptureProps) {
  const { t } = useTranslation();
  // overlays は、キャプチャに、字幕とコメントを写すかどうか。
  const [overlays, setOverlays] = useState(true);
  const [message, notify] = useNotice();

  const capture = async (action: 'save' | 'copy') => {
    const video = videoRef.current;
    const frame = frameRef.current;
    if (video === null || frame === null) {
      return;
    }
    try {
      const blob = await captureFrame(video, overlays ? overlayCanvases(frame) : []);
      if (action === 'save') {
        const stamp = new Date().toISOString().replaceAll(':', '-').replace(/\..*$/, '');
        saveBlob(blob, `${name}-${stamp}.png`);
      } else {
        await copyBlob(blob);
        notify(t('player.copied'));
      }
    } catch {
      notify(t('player.captureError'));
    }
  };

  return (
    <Tooltip label={message} opened={message !== null} withinPortal={false}>
      <div>
        <Menu position="top-end" closeOnItemClick withinPortal={false}>
          <Menu.Target>
            <ActionIcon
              variant="transparent"
              size="lg"
              aria-label={t('player.capture')}
              className={playerControlsClasses.button}
            >
              <Camera size={20} aria-hidden />
            </ActionIcon>
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
      </div>
    </Tooltip>
  );
}

// PipButton は、プレイヤーを別のウィンドウに出す操作と、このページに戻す操作 (設計書 7.5)。
export function PipButton({ pip }: { pip: ReturnType<typeof useDocumentPictureInPicture> }) {
  const { t } = useTranslation();
  const [message, notify] = useNotice();
  if (!pip.supported) {
    return null;
  }

  return (
    <Tooltip label={message} opened={message !== null} withinPortal={false}>
      <div>
        <ControlButton
          label={pip.container === null ? t('player.pip') : t('player.pipBack')}
          icon={PictureInPicture2}
          active={pip.container !== null}
          optional
          onClick={() => {
            if (pip.container === null) {
              pip.open(640, 360, classes.pip ?? '').catch(() => notify(t('player.pipError')));
            } else {
              pip.close();
            }
          }}
        />
      </div>
    </Tooltip>
  );
}
