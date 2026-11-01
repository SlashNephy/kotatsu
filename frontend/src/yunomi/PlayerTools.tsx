import { ActionIcon, Checkbox, Menu, Tooltip } from '@mantine/core';
import { Camera, PictureInPicture2 } from 'lucide-react';
import { useEffect, useState, type RefObject } from 'react';
import { useTranslation } from 'react-i18next';

import {
  captureFileName,
  captureFrame,
  captureTexts,
  copyBlob,
  overlayCanvases,
  saveBlob,
  withCaptureTexts,
} from './capture';
import { noCrop, type Crop } from './crop';
import { useCaptureFileNameTemplate } from './captureFileNameTemplate';
import { uploadCapture } from '../captures/upload';
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

type CaptureOptions = {
  frameRef: RefObject<HTMLElement | null>;
  videoRef: RefObject<HTMLVideoElement | null>;
  // serviceName と programName は、キャプチャのファイル名に使う。番組が分からなければ programName は null。
  serviceName: string;
  programName: string | null;
  // captureTime は、今の場面の時刻 (Unix ミリ秒) を返す。録画では、再生位置の放送時刻を返す。
  captureTime: () => number;
  // recordingId は、録画で撮るときの録画の ID。キャプチャの一覧から、その録画を開けるようにする (#457、#908)。
  recordingId?: string | null;
  // position は、録画で撮るときの再生位置 (録画の始まりからの秒) を返す。一覧から、撮った場面を開けるようにする。
  position?: (() => number) | null;
  // caption は、今出ている字幕の文を返す (#458)。出ていなければ null。保存する画像に書き込む。
  caption?: (() => string | null) | null;
  // crop は、今の切り抜き (#496)。キャプチャも、画面と同じ範囲で写す。
  crop?: Crop;
};

// CaptureResult は、キャプチャの結果。映像がなければ null。
export type CaptureResult = 'saved' | 'copied' | 'failed' | null;

// Capture は、キャプチャの操作と、その設定と知らせ。メニュー (CaptureButton) とキー (s) で使う。
export type Capture = {
  overlays: boolean;
  setOverlays: (overlays: boolean) => void;
  message: string | null;
  capture: (action: 'save' | 'copy') => Promise<CaptureResult>;
};

// useCapture は、今の場面を画像に保存するか、コピーする操作を返す (設計書 7.5)。
export function useCapture({
  frameRef,
  videoRef,
  serviceName,
  programName,
  captureTime,
  recordingId = null,
  position = null,
  caption = null,
  crop = noCrop,
}: CaptureOptions): Capture {
  const { t } = useTranslation();
  // overlays は、キャプチャに、字幕とコメントを写すかどうか。
  const [overlays, setOverlays] = useState(true);
  const [template] = useCaptureFileNameTemplate();
  const [message, notify] = useNotice();

  const capture = async (action: 'save' | 'copy'): Promise<CaptureResult> => {
    const video = videoRef.current;
    const frame = frameRef.current;
    if (video === null || frame === null) {
      return null;
    }
    // 画像を作る間に再生が進むので、押した時の時刻を先に取る。
    const takenAt = Date.now();
    const time = captureTime();
    const at = position?.() ?? null;
    const text = caption?.() ?? null;
    try {
      const blob = await captureFrame(video, overlays ? overlayCanvases(frame) : [], crop);
      // 番組情報を書き込んだ画像を、保存してもコピーしても、サーバーに残す (#908)。残せなくても、保存とコピーは続ける。
      const named = await withCaptureTexts(
        blob,
        captureTexts(time, serviceName, programName, text),
      );
      // uploadFailed は、サーバーに残せなかったか。先に失敗を知らせていたら、コピーできた知らせで上書きしない。
      let uploadFailed = false;
      uploadCapture(named, {
        time,
        takenAt,
        serviceName,
        programName,
        recordingId,
        position: at,
        caption: text,
      }).catch(() => {
        uploadFailed = true;
        notify(t('player.captureUploadError'));
      });
      if (action === 'save') {
        saveBlob(named, captureFileName(time, serviceName, programName, template));

        return 'saved';
      }
      // クリップボードでは、ブラウザが文字の情報を落とすことがあるので、書き込む前の画像を写す。
      await copyBlob(blob);
      if (!uploadFailed) {
        notify(t('player.copied'));
      }

      return 'copied';
    } catch {
      notify(t('player.captureError'));

      return 'failed';
    }
  };

  return { overlays, setOverlays, message, capture };
}

// CaptureButton は、今の場面を画像に保存するか、コピーするメニュー (設計書 7.5)。
// 別のウィンドウに移っても開けるよう、メニューと知らせはプレイヤーの中に描く。
export function CaptureButton({
  capture: { overlays, setOverlays, message, capture },
}: {
  capture: Capture;
}) {
  const { t } = useTranslation();

  return (
    <Menu position="top-end" closeOnItemClick withinPortal={false}>
      <Menu.Target>
        <Tooltip
          label={message ?? t('player.capture')}
          opened={message === null ? undefined : true}
          withinPortal={false}
        >
          <ActionIcon
            variant="transparent"
            size="lg"
            aria-label={t('player.capture')}
            className={playerControlsClasses.button}
          >
            <Camera size={20} aria-hidden />
          </ActionIcon>
        </Tooltip>
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
  );
}

// openPictureInPicture は、視聴画面を別のウィンドウに出す。
export function openPictureInPicture(
  pip: ReturnType<typeof useDocumentPictureInPicture>,
): Promise<void> {
  return pip.open(640, 360, classes.pip ?? '');
}

// PipButton は、プレイヤーを別のウィンドウに出す操作と、このページに戻す操作 (設計書 7.5)。
export function PipButton({ pip }: { pip: ReturnType<typeof useDocumentPictureInPicture> }) {
  const { t } = useTranslation();
  const [message, notify] = useNotice();
  if (!pip.supported) {
    return null;
  }

  return (
    <ControlButton
      label={pip.container === null ? t('player.pip') : t('player.pipBack')}
      icon={PictureInPicture2}
      active={pip.container !== null}
      optional
      notice={message}
      onClick={() => {
        if (pip.container === null) {
          openPictureInPicture(pip).catch(() => notify(t('player.pipError')));
        } else {
          pip.close();
        }
      }}
    />
  );
}
