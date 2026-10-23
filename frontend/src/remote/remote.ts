import { RemoteKey } from '../yunomi/useDataBroadcast';

// estimatedPosition は、ほかの端末の、今の再生位置 (秒) を見積もる。プレイヤーは状態を 10 秒ごとにしか送らないので、
// 再生中なら、送った時刻からの経過を足す。一時停止中か、位置が分からなければ、送った位置のままにする。長さを超えない。
export function estimatedPosition(
  state: {
    position: number | null | undefined;
    duration: number | null | undefined;
    paused: boolean;
  },
  lastSeenAt: string,
  now: number,
): number | null {
  if (state.position == null) {
    return null;
  }
  const elapsed = state.paused ? 0 : Math.max(0, (now - Date.parse(lastSeenAt)) / 1000);
  const position = state.position + elapsed;

  return state.duration == null ? position : Math.min(position, state.duration);
}

// remoteKeyName は、データ放送のリモコンのボタンを、操作で送る名前にする。
export function remoteKeyName(key: RemoteKey): string | null {
  const entry = Object.entries(RemoteKey).find(([, value]) => value === key);

  return entry === undefined ? null : entry[0];
}
