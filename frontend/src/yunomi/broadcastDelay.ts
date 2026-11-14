// 映しているコマが、放送からどれだけ遅れているかを出す (#1515、設計書 7.5)。
// tenban が、PCR と、それを含む TS を受け取った時刻の組を配る。映しているコマの放送の時刻は、
// 組の受け取った時刻 + (コマの PTS − 組の PCR) で出せる。端末の時計は当てにせず、配られたサーバーの時計との差で直す。

// ptsWrapSeconds は、PTS と PCR の基準部 (33 ビット、90 kHz) が一周する長さ (秒)。
const ptsWrapSeconds = 2 ** 33 / 90_000;

export type BroadcastClockSample = {
  // pcr は、PCR の基準部を秒にしたもの。
  pcr: number;
  // receivedAt は、PCR を含む TS を tenban が受け取った時刻 (サーバーの時計、ミリ秒)。
  receivedAt: number;
  // serverTime は、配った時点のサーバーの時計 (ミリ秒)。
  serverTime: number;
  // arrivedAt は、配られたものが端末に届いた時刻 (端末の時計、ミリ秒)。LAN の片道は小さいので無視する。
  arrivedAt: number;
};

// broadcastDelaySeconds は、画面の時刻 mediaTime (秒) のコマが、放送から何秒遅れているかを返す。
// mediaOrigin は、画面の時刻 0 に当たる放送の時刻 (90 kHz の刻み)。now は、端末の今の時刻 (ミリ秒)。
export function broadcastDelaySeconds(
  mediaTime: number,
  mediaOrigin: number,
  clock: BroadcastClockSample,
  now: number,
): number {
  const pts = (mediaOrigin / 90_000 + mediaTime) % ptsWrapSeconds;
  // PTS と PCR は一周するので、差を、一周の半分の内に寄せる。
  let difference = (((pts - clock.pcr) % ptsWrapSeconds) + ptsWrapSeconds) % ptsWrapSeconds;
  if (difference > ptsWrapSeconds / 2) {
    difference -= ptsWrapSeconds;
  }
  const broadcastAt = clock.receivedAt + difference * 1_000;
  const serverNow = now + (clock.serverTime - clock.arrivedAt);

  return (serverNow - broadcastAt) / 1_000;
}
