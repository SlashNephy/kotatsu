// IdP での再認証を、別のウィンドウで行うための仕組み (#916)。
// 元の画面を離れないので、再認証を待っている操作 (React の状態にしか持てない) を、終えた後に続けられる。
// IdP から戻ったウィンドウは、元の画面へ結果を BroadcastChannel で知らせる。
// IdP が Cross-Origin-Opener-Policy を付けると window.opener が切れるので、opener には頼らない。

// oidcReauthDonePath は、別のウィンドウで再認証を終えたときに戻す画面のパス。
export const oidcReauthDonePath = '/reauth/done';

const channelName = 'kotatsu-oidc-reauth';
const windowName = 'kotatsu-oidc-reauth';

// OIDCReauthResult は、別のウィンドウでの再認証の結果。id で、どの再認証の結果かを見分ける。
// error は、失敗したときの理由 (oidc_denied など)。
export type OIDCReauthResult = { id: string; error: string | undefined };

// openOIDCReauthWindow は、再認証のための別のウィンドウを開く。開けなければ (ポップアップを止められたら) null を返す。
// ブラウザーがポップアップを止めないよう、押した操作の中で、IdP の URL を待たずに呼ぶ。
export function openOIDCReauthWindow(): Window | null {
  return window.open('', windowName, 'popup,width=520,height=720');
}

// oidcReauthReturnTo は、別のウィンドウで再認証を終えたときに戻すパス。
export function oidcReauthReturnTo(id: string): string {
  return `${oidcReauthDonePath}?${new URLSearchParams({ id }).toString()}`;
}

// waitForOIDCReauthentication は、id の再認証の結果を待ち、届いたら onResult を呼ぶ。返した関数で、待つのをやめる。
export function waitForOIDCReauthentication(
  id: string,
  onResult: (result: OIDCReauthResult) => void,
): () => void {
  const channel = new BroadcastChannel(channelName);
  channel.onmessage = (event: MessageEvent<OIDCReauthResult>) => {
    if (event.data.id === id) {
      onResult(event.data);
    }
  };

  return () => channel.close();
}

// reportOIDCReauthentication は、再認証の結果を、待っている元の画面に知らせる。
export function reportOIDCReauthentication(result: OIDCReauthResult) {
  const channel = new BroadcastChannel(channelName);
  channel.postMessage(result);
  channel.close();
}
