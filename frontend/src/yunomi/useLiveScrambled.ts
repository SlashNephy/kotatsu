import { useMemo, useRef } from 'react';
import { graphql, useSubscription } from 'react-relay';

import type { useLiveScrambledSubscription } from './__generated__/useLiveScrambledSubscription.graphql';

// useLiveScrambled は、ライブのサービス serviceId が、スクランブルされたまま届くと決まったら onScrambled を呼ぶ (設計書 7.1、#1517)。
// 決めるのは、ストリームを中継する tenban か kyusu で、決めたらストリームを閉じるので、プレイヤーはつなぎ直すのをやめて理由を出す。
// 購読を始めた後に決まったものだけが届く。
export function useLiveScrambled(serviceId: string, onScrambled: () => void) {
  // 購読し直さないよう、最新の関数を参照で持つ。
  const callback = useRef(onScrambled);
  callback.current = onScrambled;
  const config = useMemo(
    () => ({
      subscription: graphql`
        subscription useLiveScrambledSubscription($service: ID!) {
          liveStreamScrambled(service: $service) {
            decidedAt
          }
        }
      `,
      variables: { service: serviceId },
      onNext: () => callback.current(),
      // 購読できなくても、再生は続ける。理由が出ないだけで、切れたときはこれまでどおりつなぎ直す。
      onError: () => {},
    }),
    [serviceId],
  );
  useSubscription<useLiveScrambledSubscription>(config);
}
