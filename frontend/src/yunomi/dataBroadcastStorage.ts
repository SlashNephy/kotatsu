import { bmlStoragePrefix, zipcodeNVRAMKey } from './dataBroadcastZipcode';

// clearDataBroadcastStorage は、データ放送の文書がこの端末に保存したデータ (NVRAM など) を消し、消した件数を返す (設計書 7.4、#502)。
// 局の文書が壊れた値を読み続けて表示がおかしいままになったときに、やり直せるようにする。
// web-bml の prefix の下のキーだけを消し、受信機の設定である郵便番号は残す。
export function clearDataBroadcastStorage(
  storage: Pick<Storage, 'length' | 'key' | 'removeItem'> | null,
): number {
  if (storage === null) {
    return 0;
  }
  try {
    const keys: string[] = [];
    for (let i = 0; i < storage.length; i++) {
      const key = storage.key(i);
      if (key?.startsWith(bmlStoragePrefix) === true && key !== zipcodeNVRAMKey) {
        keys.push(key);
      }
    }
    // 消しながら数えると番号がずれるので、集めてから消す。
    for (const key of keys) {
      storage.removeItem(key);
    }

    return keys.length;
  } catch {
    return 0;
  }
}
