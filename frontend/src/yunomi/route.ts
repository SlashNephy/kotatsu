// 再生の経路を選ぶ (設計書 7.1)。端末の能力と、サーバー側でのトランスコード (kyusu) の有無と、放送の映像の種類から決める。

// Capabilities は、端末で使える再生の仕組み。
export type Capabilities = {
  // convert は、ブラウザの中で MPEG-2 を H.264 に変換して再生できるかどうか。
  // MediaSource で H.264 を再生できる必要がある。iPhone のように ManagedMediaSource しかない端末では使わない。
  convert: boolean;
  // hls は、kyusu の HLS を hls.js で再生できるかどうか。
  hls: boolean;
  // hevc は、HEVC の映像を再生できるかどうか。できなければ、HEVC の画質を選ばせない。
  hevc: boolean;
  // hevcUhd は、BS4K の映像 (HEVC Main10、2160p) をそのまま再生できるかどうか (#806)。
  // できなければ、映像を変換しない画質を選ばせない。
  hevcUhd: boolean;
};

// passthrough は、映像を変換しない (BS4K の HEVC をそのまま送る) 画質かどうか。
export type Preset = { name: string; codec: string; passthrough: boolean };

export type Transcoding = { available: boolean; presets: readonly Preset[] };

// VideoFormat は、放送の映像の種類 (#806)。HD は 2K 以下、UHD は BS4K、SHV は BS8K。
export type VideoFormat = 'HD' | 'UHD' | 'SHV';

// toVideoFormat は、GraphQL の映像の種類を VideoFormat にする。知らない値 (サーバーが新しい) なら、
// ブラウザの中での変換を選ばせないよう、BS4K と同じく扱う。
export function toVideoFormat(value: string): VideoFormat {
  return value === 'HD' || value === 'SHV' ? value : 'UHD';
}

// original は、ブラウザの中で変換する経路を表す画質の名前。
export const original = 'original';

export type PlaybackRoute =
  | { kind: 'convert' }
  | { kind: 'kyusu'; preset: string }
  // unavailable は、再生できないこと。reason は、利用者に示す理由。
  | { kind: 'unavailable'; reason: 'noTranscoder' | 'noPlayer' | 'uhdNoTranscoder' | 'shv' };

// defaultPreset は、kyusu の経路を選んだときの既定の画質。スマートフォンの回線を想定して控えめにする。
const defaultPreset = '720p';
// uhdDesktopPreset は、BS4K の映像をそのまま再生できない PC で、kyusu の経路を選んだときの既定の画質。
// 2K の番組ではブラウザの中で変換した元の画質で見られるので、4K の番組でも画質を落としすぎない。
const uhdDesktopPreset = '1080p';

// qualities は、画質のメニューに出す選択肢を返す。HD の番組では、先頭が、ブラウザの中での変換 (オリジナル)。
// BS4K の映像は MPEG-2 でないので、ブラウザの中では変換できず、kyusu の画質だけを出す。映像を変換しない画質は、BS4K の番組で、
// 端末がそのまま再生できるときだけ出す。8K の番組は、どの経路でも再生できない (#806)。
export function qualities(
  capabilities: Capabilities,
  transcoding: Transcoding,
  format: VideoFormat,
): string[] {
  const list: string[] = [];
  if (format === 'SHV') {
    return list;
  }
  if (format === 'HD' && capabilities.convert) {
    list.push(original);
  }
  if (transcoding.available && capabilities.hls) {
    for (const preset of transcoding.presets) {
      if (preset.passthrough) {
        if (format === 'UHD' && capabilities.hevcUhd) {
          list.push(preset.name);
        }
        continue;
      }
      if (preset.codec !== 'HEVC' || capabilities.hevc) {
        list.push(preset.name);
      }
    }
  }

  return list;
}

// chooseRoute は、再生の経路を返す。requested は、利用者が画質のメニューで選んだもの。選べないものなら、既定に戻す。
// 既定では、サーバーに負荷をかけない経路を選ぶ。HD の番組ではブラウザの中での変換、BS4K の番組では映像を変換しない画質を選び、
// 使えない端末では kyusu で変換する。
export function chooseRoute(
  capabilities: Capabilities,
  transcoding: Transcoding,
  requested: string | undefined,
  format: VideoFormat,
): PlaybackRoute {
  if (format === 'SHV') {
    return { kind: 'unavailable', reason: 'shv' };
  }
  const choices = qualities(capabilities, transcoding, format);
  if (requested !== undefined && choices.includes(requested)) {
    return requested === original ? { kind: 'convert' } : { kind: 'kyusu', preset: requested };
  }
  if (choices.includes(original)) {
    return { kind: 'convert' };
  }
  const passthrough = transcoding.presets.find(
    (preset) => preset.passthrough && choices.includes(preset.name),
  );
  if (passthrough !== undefined) {
    return { kind: 'kyusu', preset: passthrough.name };
  }
  const preferred = format === 'UHD' && capabilities.convert ? uhdDesktopPreset : defaultPreset;
  const preset = choices.includes(preferred) ? preferred : choices[0];
  if (preset !== undefined) {
    return { kind: 'kyusu', preset };
  }

  // 再生できる経路がない。kyusu がいれば使えるのか、端末が HLS も再生できないのかで、理由を分ける。
  // BS4K の番組は、ブラウザの中で変換できないので、kyusu がいなければ、どの端末でも再生できない。
  if (transcoding.available) {
    return { kind: 'unavailable', reason: 'noPlayer' };
  }

  return { kind: 'unavailable', reason: format === 'UHD' ? 'uhdNoTranscoder' : 'noTranscoder' };
}

// KyusuAudio は、kyusu の HLS に入れる音声。1 は第 1 音声 (主音声)、2 は第 2 音声 (副音声や 2 つ目の音声)。
export type KyusuAudio = 1 | 2;

// hlsURL は、kyusu の HLS のプレイリストの URL を返す。streamUrl は、tenban の TS の URL。
// TS の URL にクエリがあれば、プレイリストの URL の後ろに付け直す。
export function hlsURL(streamUrl: string, preset: string, audio: KyusuAudio): string {
  const [path = '', query] = streamUrl.split('?', 2);
  const playlist = `${path}/hls/${encodeURIComponent(preset)}/${audio}/playlist.m3u8`;

  return query === undefined ? playlist : `${playlist}?${query}`;
}

// serviceIDBase は、Mirakurun のサービス ID を組み立てる基数 (networkId × 100000 + serviceId)。
const serviceIDBase = 100_000;

// DataBroadcastSource は、データ放送の TS を読む先と、その TS の中のサービスの program_number。
export type DataBroadcastSource = { url: string; serviceId: number };

// dataBroadcastSource は、ライブ視聴の TS の URL から、データ放送用のエンドポイントを返す (設計書 7.4)。
// URL の最後の部分が、Mirakurun のサービス ID でなければ null を返す。
export function dataBroadcastSource(streamUrl: string): DataBroadcastSource | null {
  const [path = '', query] = streamUrl.split('?', 2);
  const id = Number(path.split('/').pop());
  if (!Number.isSafeInteger(id) || id <= 0) {
    return null;
  }
  const url = `${path}/data-broadcast`;

  return { url: query === undefined ? url : `${url}?${query}`, serviceId: id % serviceIDBase };
}
