import { useSyncExternalStore } from 'react';

import { browserStorage } from './volume';

// データ放送の受信機の郵便番号を、端末ごとに覚えておく (設計書 7.4、#442)。
// 天気やニュースには、郵便番号で地域を選ぶ文書がある。自宅の外から見ることもあるので、アカウントには保存しない。既定は空。

const storageKey = 'kotatsu.player.dataBroadcastZipcode';
// changeEvent は、同じタブの中で変えたことを知らせるイベント。storage イベントは、ほかのタブにしか届かない。
const changeEvent = 'kotatsu:data-broadcast-zipcode';

// bmlStoragePrefix は、web-bml が localStorage に書くキーの prefix。Kotatsu 自身のキーと名前空間を分ける。
export const bmlStoragePrefix = 'kotatsu.bml.';
// zipcodeNVRAMKey は、web-bml が受信機の郵便番号 (nvram://receiverinfo/zipcode) を読むキー。web-bml の既定の nvramPrefix に合わせる。
export const zipcodeNVRAMKey = `${bmlStoragePrefix}nvram_prefix=receiverinfo%2Fzipcode`;

// normalizeZipcode は、入力を 7 桁の半角数字にする。全角の数字とハイフンは受け付ける。空なら空を、読めなければ null を返す。
export function normalizeZipcode(input: string): string | null {
  const digits = input
    .trim()
    .replace(/[０-９]/g, (c) => String.fromCharCode(c.charCodeAt(0) - 0xfee0))
    .replace(/[-－ー‐]/g, '');
  if (digits === '') {
    return '';
  }

  return /^\d{7}$/.test(digits) ? digits : null;
}

// loadDataBroadcastZipcode は、覚えた郵便番号を読む。読めなければ、既定の空を返す。
export function loadDataBroadcastZipcode(storage: Pick<Storage, 'getItem'> | null): string {
  try {
    return normalizeZipcode(storage?.getItem(storageKey) ?? '') ?? '';
  } catch {
    return '';
  }
}

// saveDataBroadcastZipcode は、郵便番号を覚える。空なら忘れる。書けなくても、例外は投げない。
export function saveDataBroadcastZipcode(
  storage: Pick<Storage, 'setItem' | 'removeItem'> | null,
  zipcode: string,
): void {
  try {
    if (zipcode === '') {
      storage?.removeItem(storageKey);
    } else {
      storage?.setItem(storageKey, zipcode);
    }
  } catch {
    // 書けなければ、次に開いたときは既定に戻るだけ。
  }
}

// writeZipcodeNVRAM は、郵便番号を、web-bml が読む NVRAM に書く。web-bml は値を base64 で持つ。空なら消す。
// BMLBrowser を作る前に呼ぶ。
export function writeZipcodeNVRAM(
  storage: Pick<Storage, 'setItem' | 'removeItem'> | null,
  zipcode: string,
): void {
  try {
    if (zipcode === '') {
      storage?.removeItem(zipcodeNVRAMKey);
    } else {
      storage?.setItem(zipcodeNVRAMKey, btoa(zipcode));
    }
  } catch {
    // 書けなければ、文書は郵便番号が設定されていないものとして扱う。
  }
}

function subscribe(onChange: () => void): () => void {
  window.addEventListener('storage', onChange);
  window.addEventListener(changeEvent, onChange);

  return () => {
    window.removeEventListener('storage', onChange);
    window.removeEventListener(changeEvent, onChange);
  };
}

// useDataBroadcastZipcode は、郵便番号と、それを変える関数を返す。ほかのタブで変えても追う。
export function useDataBroadcastZipcode(): [string, (zipcode: string) => void] {
  const zipcode = useSyncExternalStore(subscribe, () => loadDataBroadcastZipcode(browserStorage()));

  return [
    zipcode,
    (next) => {
      saveDataBroadcastZipcode(browserStorage(), next);
      window.dispatchEvent(new Event(changeEvent));
    },
  ];
}
