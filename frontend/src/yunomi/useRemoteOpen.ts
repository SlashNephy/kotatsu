import { useNavigate } from '@tanstack/react-router';
import { useCallback } from 'react';

import type { RemoteTarget } from './useRemotePlayer';

// useRemoteOpen は、ほかの端末から開かせられたもの (キャスト、#802) の画面へ移る関数を返す。
// サービスならライブ視聴、録画なら録画の再生を開く。音ありの自動再生を断られたら、移った先のプレイヤーが再生のボタンを出す。
export function useRemoteOpen(): (target: RemoteTarget) => void {
  const navigate = useNavigate();

  return useCallback(
    (target: RemoteTarget) => {
      if (target.kind === 'LIVE') {
        void navigate({ to: '/live/$serviceId', params: { serviceId: target.serviceId } });
      } else {
        void navigate({
          to: '/recordings/$recordingId/play',
          params: { recordingId: target.recordingId },
        });
      }
    },
    [navigate],
  );
}
