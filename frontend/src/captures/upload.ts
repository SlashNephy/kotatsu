// キャプチャをサーバーに残す (#908、設計書 7.5)。撮ったキャプチャは、保存してもコピーしても、撮ったユーザーのものとして送る。
// 残したキャプチャは、ユーザーのメニューのキャプチャの一覧から、どの端末でも見られる。

// uploadPath は、キャプチャを受け取る tenban のパス。
export const uploadPath = '/media/captures';

// CaptureInfo は、撮った場面の情報。time は撮った場面の放送時刻、takenAt は撮った時刻 (どちらも Unix ミリ秒)。
export type CaptureInfo = {
  time: number;
  takenAt: number;
  serviceName: string;
  programName: string | null;
  // recordingId は、録画で撮ったときの録画の ID。position は、そのときの再生位置 (録画の始まりからの秒)。ライブなら、どちらも null。
  recordingId: string | null;
  position: number | null;
  // caption は、撮ったときに出ていた字幕の文 (#458)。出ていなければ null。
  caption: string | null;
};

// captureMetadata は、撮った場面の情報を、tenban に送る JSON の形にする。日時は RFC 3339 で送る。
// 録画でなければ、再生位置は送らない (tenban は、録画のない再生位置を断る)。
export function captureMetadata(info: CaptureInfo) {
  return {
    serviceName: info.serviceName,
    programName: info.programName,
    caption: info.caption,
    recordingId: info.recordingId,
    position: info.recordingId === null ? null : info.position,
    broadcastAt: new Date(info.time).toISOString(),
    takenAt: new Date(info.takenAt).toISOString(),
  };
}

// uploadCapture は、PNG の画像 image を、撮った場面の情報と一緒に、サーバーに残す。残せなければ例外を投げる。
export async function uploadCapture(image: Blob, info: CaptureInfo): Promise<void> {
  const body = new FormData();
  body.append(
    'metadata',
    new Blob([JSON.stringify(captureMetadata(info))], { type: 'application/json' }),
  );
  body.append('image', image, 'capture.png');
  const response = await fetch(uploadPath, { method: 'POST', credentials: 'same-origin', body });
  if (!response.ok) {
    throw new Error(`failed to upload the capture: ${response.status}`);
  }
}

// legacyDatabaseName は、キャプチャの履歴を端末に残していた版 (#457) の IndexedDB の名前。
const legacyDatabaseName = 'kotatsu-captures';

// removeLegacyHistory は、端末に残していたキャプチャの履歴を消す。サーバーの一覧に置き換えたので、移さずに捨てる (#908)。
// 消せなくても (IndexedDB を使えないブラウザなど)、何もしない。
export function removeLegacyHistory(): void {
  try {
    indexedDB.deleteDatabase(legacyDatabaseName);
  } catch {
    // 使えなければ、残っていないので、消すものもない。
  }
}
