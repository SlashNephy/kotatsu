// 再生の経路を選ぶ (設計書 7.1)。端末の能力と、サーバー側でのトランスコード (kyusu) の有無から決める。

// Capabilities は、端末で使える再生の仕組み。
export type Capabilities = {
  // convert は、ブラウザの中で MPEG-2 を H.264 に変換して再生できるかどうか。
  // MediaSource で H.264 を再生できる必要がある。iPhone のように ManagedMediaSource しかない端末では使わない。
  convert: boolean;
  // hls は、kyusu の HLS を hls.js で再生できるかどうか。
  hls: boolean;
  // hevc は、HEVC の映像を再生できるかどうか。できなければ、HEVC の画質を選ばせない。
  hevc: boolean;
};

export type Preset = { name: string; codec: string };

export type Transcoding = { available: boolean; presets: readonly Preset[] };

// original は、ブラウザの中で変換する経路を表す画質の名前。
export const original = 'original';

export type PlaybackRoute =
  | { kind: 'convert' }
  | { kind: 'kyusu'; preset: string }
  // unavailable は、再生できないこと。reason は、利用者に示す理由。
  | { kind: 'unavailable'; reason: 'noTranscoder' | 'noPlayer' };

// defaultPreset は、kyusu の経路を選んだときの既定の画質。スマートフォンの回線を想定して控えめにする。
const defaultPreset = '720p';

// qualities は、画質のメニューに出す選択肢を返す。先頭が、ブラウザの中での変換 (オリジナル)。
export function qualities(capabilities: Capabilities, transcoding: Transcoding): string[] {
  const list: string[] = [];
  if (capabilities.convert) {
    list.push(original);
  }
  if (transcoding.available && capabilities.hls) {
    for (const preset of transcoding.presets) {
      if (preset.codec !== 'HEVC' || capabilities.hevc) {
        list.push(preset.name);
      }
    }
  }

  return list;
}

// chooseRoute は、再生の経路を返す。requested は、利用者が画質のメニューで選んだもの。選べないものなら、既定に戻す。
// 既定では、サーバーに負荷をかけないブラウザの中での変換を選び、使えない端末では kyusu を選ぶ。
export function chooseRoute(
  capabilities: Capabilities,
  transcoding: Transcoding,
  requested: string | undefined,
): PlaybackRoute {
  const choices = qualities(capabilities, transcoding);
  if (requested !== undefined && choices.includes(requested)) {
    return requested === original ? { kind: 'convert' } : { kind: 'kyusu', preset: requested };
  }
  if (capabilities.convert) {
    return { kind: 'convert' };
  }
  const preset = choices.includes(defaultPreset) ? defaultPreset : choices[0];
  if (preset !== undefined) {
    return { kind: 'kyusu', preset };
  }

  // ブラウザの中で変換できず、kyusu も使えない。kyusu がいれば使えるのか、端末が HLS も再生できないのかで、理由を分ける。
  return {
    kind: 'unavailable',
    reason: transcoding.available ? 'noPlayer' : 'noTranscoder',
  };
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
