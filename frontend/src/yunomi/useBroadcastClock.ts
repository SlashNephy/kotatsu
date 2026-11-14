import { useEffect, useRef } from 'react';
import { graphql, useRelayEnvironment } from 'react-relay';
import { requestSubscription } from 'relay-runtime';

import type { BroadcastNow } from '../jikkyo/liveSync';
import type { useBroadcastClockSubscription } from './__generated__/useBroadcastClockSubscription.graphql';
import { broadcastDelaySeconds, type BroadcastClockSample } from './broadcastDelay';
import type { StreamMeter } from './streamMeter';

// maxDelaySeconds は、放送からの遅れとして使う上限。これを超える値は、組と画面の時刻の取り違えとみなして使わない。
const maxDelaySeconds = 3_600;

// useBroadcastClock は、ライブのサービス serviceId の放送の時計を tenban から受け取り、映しているコマの放送からの遅れを返す関数を返す (#1515)。
// 実況のコメントを、映している場面の時刻に合わせるのに使う。serviceId が null の間は受け取らない。
// 返す関数は、映像、画面の時刻の起点 (ブラウザの中で変換する経路と、kyusu の経路のライブが知らせる)、放送の時計のどれかがなければ null を返す。
export function useBroadcastClock(
  serviceId: string | null,
  video: HTMLVideoElement | null,
  meter: StreamMeter,
): () => BroadcastNow | null {
  const environment = useRelayEnvironment();
  const sample = useRef<BroadcastClockSample | null>(null);
  const videoRef = useRef(video);
  videoRef.current = video;

  useEffect(() => {
    sample.current = null;
    if (serviceId === null) {
      return;
    }
    const disposable = requestSubscription<useBroadcastClockSubscription>(environment, {
      subscription: graphql`
        subscription useBroadcastClockSubscription($service: ID!) {
          liveStreamRates(service: $service) {
            broadcastClock {
              pcr
              receivedAt
            }
            serverTime
          }
        }
      `,
      variables: { service: serviceId },
      onNext: (response) => {
        const rates = response?.liveStreamRates;
        sample.current =
          rates?.broadcastClock == null
            ? null
            : {
                pcr: rates.broadcastClock.pcr,
                receivedAt: Date.parse(rates.broadcastClock.receivedAt),
                serverTime: Date.parse(rates.serverTime),
                arrivedAt: Date.now(),
              };
      },
      // 受け取れなければ、受け取った時刻で流す。
      onError: () => {
        sample.current = null;
      },
    });

    return () => {
      disposable.dispose();
      sample.current = null;
    };
  }, [environment, serviceId]);

  return () => {
    const clock = sample.current;
    const current = videoRef.current;
    const origin = meter.mediaOrigin;
    if (clock === null || current === null || origin === null) {
      return null;
    }
    const now = Date.now();
    const delay = broadcastDelaySeconds(current.currentTime, origin, clock, now);
    if (Math.abs(delay) > maxDelaySeconds) {
      return null;
    }

    return { delay, serverNow: now + (clock.serverTime - clock.arrivedAt) };
  };
}
