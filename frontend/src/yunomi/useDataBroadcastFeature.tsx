import { type RefObject, useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { PlayerCommands } from './commands';
import { useDataBroadcastKeys } from './dataBroadcastKeys';
import { useDataBroadcastPreload } from './dataBroadcastPreload';
import type { PlayerMedia } from './playerMedia';
import type { DataBroadcastSource } from './route';
import { type DataBroadcastHandlers, RemoteKey, useDataBroadcast } from './useDataBroadcast';

// データ放送 (設計書 7.4) を扱う機能の部品 (#2244)。出し入れ、プリロード、リモコンの操作、キーの受け付けをまとめる。
// 再生元は、データ放送の TS を流す配信と、今受け取ってよいか (映像が見えていて、視聴が切れていない) を渡す。

// DataBroadcastFeatureOptions は、再生元から受け取る、データ放送の扱い。
export type DataBroadcastFeatureOptions = {
  // source は、データ放送の TS を流す配信。データ放送のない映像 (HD でないもの) では null。
  source: DataBroadcastSource | null;
  // receiving は、今データ放送の TS を受け取ってよいか。視聴が切れている間と、映像が見えていない間は受け取らない。
  receiving: boolean;
  mediaRef: RefObject<PlayerMedia | null>;
  // ipEnabled は、管理者が、文書の双方向の通信を tenban に中継させているか (#501)。
  ipEnabled: boolean;
  handlers: DataBroadcastHandlers;
  showFlash: (text: string) => void;
};

// DataBroadcastFeature は、useDataBroadcastFeature が返す、データ放送の状態と操作。
export type DataBroadcastFeature = ReturnType<typeof useDataBroadcast> & {
  // enabled は、データ放送を出しているかどうか。映像を覆うので、既定では出さない。
  enabled: boolean;
  // commands は、データ放送の出し入れと、リモコンのボタンの操作。データ放送のない映像では、どちらも使えない。
  commands: PlayerCommands;
};

// useDataBroadcastFeature は、データ放送を用意する。プリロードする設定か、一度出したあとは、出していない間も裏で受け取り、
// 出したらすぐに d ボタンで入れるようにする。
export function useDataBroadcastFeature({
  source,
  receiving,
  mediaRef,
  ipEnabled,
  handlers,
  showFlash,
}: DataBroadcastFeatureOptions): DataBroadcastFeature {
  const { t } = useTranslation();
  const [enabled, setEnabled] = useState(false);
  const [preload] = useDataBroadcastPreload();
  // used は、このページでデータ放送を出したことがあるか。一度出したら、消しても受け取りを続け、次はすぐに出す。
  const [used, setUsed] = useState(false);
  if (enabled && !used) {
    setUsed(true);
  }
  const dataBroadcast = useDataBroadcast(
    source !== null && (preload || used) && receiving ? source : null,
    mediaRef,
    enabled,
    ipEnabled,
    handlers,
  );
  // press は、リモコンのボタンを押す。データ放送を出していなければ、d ボタンで出す。ほかのボタンは、出している間だけ渡す。
  // データ放送のない映像では、出すと閉じられなくなるので、何もしない。
  const press = (key: RemoteKey) => {
    if (source === null) {
      return;
    }
    if (!enabled) {
      if (key === RemoteKey.d) {
        setEnabled(true);
      }

      return;
    }
    dataBroadcast.press(key);
  };
  // データ放送を出している間は、⌥+矢印、Enter、Backspace、数字、⌥+b/r/g/y でリモコンを押せる (#441)。
  useDataBroadcastKeys(
    enabled && dataBroadcast.state === 'shown' && !dataBroadcast.receiving,
    dataBroadcast.keyGroups,
    press,
  );

  return {
    ...dataBroadcast,
    press,
    enabled,
    commands:
      source === null
        ? {}
        : {
            toggleDataBroadcast: () => {
              setEnabled(!enabled);
              showFlash(
                t(enabled ? 'player.flash.dataBroadcastOff' : 'player.flash.dataBroadcastOn'),
              );
            },
            pressKey: press,
          },
  };
}
