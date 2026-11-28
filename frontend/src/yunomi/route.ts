// 再生の経路を選ぶ (設計書 7.1)。端末の能力と、サーバー側でのトランスコード (kyusu) の有無と、放送の映像の種類から決める。
import { demoStreamURL } from '../demo/streamURL';

// Capabilities は、端末で使える再生の仕組み。
export type Capabilities = {
  // convert は、ブラウザの中で MPEG-2 を H.264 に変換して再生できるかどうか。
  // MediaSource か ManagedMediaSource (iPhone、#1256) で H.264 を再生できる必要がある。
  convert: boolean;
  // hls は、kyusu の HLS を hls.js で再生できるかどうか。
  hls: boolean;
  // hevc は、HEVC の映像を再生できるかどうか。できなければ、HEVC の画質を選ばせない。
  hevc: boolean;
  // hevcUhd は、BS4K の映像 (HEVC Main10、2160p) をそのまま再生できるかどうか (#806)。
  // できなければ、映像を変換しない画質を選ばせない。
  hevcUhd: boolean;
  // decode は、ブラウザの中で MPEG-2 をデコードして再生できるかどうか (#1516)。WASM の SIMD、worker の OffscreenCanvas、
  // GPU で描く WebGL2、AudioContext が要る。
  decode: boolean;
  // fork は、評価のために並べるフォークのプレイヤー (#2287) でデコードして再生できるかどうか。decode の条件に加えて、
  // 音を鳴らす AudioWorklet が要るので、安全な文脈 (https か localhost) でだけ使える。
  fork?: boolean;
};

// passthrough は、映像を変換しない (BS4K の HEVC をそのまま送る) 画質かどうか。
// videoBitrate は、映像の平均のビットレート (kbps、設計書 7.3)。分からなければ省く。
export type Preset = { name: string; codec: string; passthrough: boolean; videoBitrate?: number };

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
// decoded は、ブラウザの中でデコードする経路 (#1516) を表す画質の名前。手で選んだときだけ使い、自動では選ばない。
export const decoded = 'decoded';
// forked は、フォークのプレイヤーでデコードする経路 (#2287) を表す画質の名前。評価のために並べ、手で選んだときだけ使う。
export const forked = 'forked';

// RouteOptions は、画面ごとに選べる経路。
export type RouteOptions = {
  // decode は、ブラウザの中でデコードする経路を選ばせるか。ライブ視聴、録画の再生、端末に保存した録画の再生で選ばせる (#2166)。
  decode?: boolean;
};

export type PlaybackRoute =
  // engine は、プレイヤーの種類。fork なら、評価のために並べるフォークで再生する (#2287)。省けば otya128/mpeg2toh264。
  | { kind: 'convert'; engine?: 'fork' }
  | { kind: 'decode' }
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
  options: RouteOptions = {},
): string[] {
  const list: string[] = [];
  if (format === 'SHV') {
    return list;
  }
  if (format === 'HD' && capabilities.convert) {
    list.push(original);
  }
  if (format === 'HD' && capabilities.decode && options.decode === true) {
    list.push(decoded);
  }
  if (format === 'HD' && capabilities.fork === true && options.decode === true) {
    list.push(forked);
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
  options: RouteOptions = {},
): PlaybackRoute {
  if (format === 'SHV') {
    return { kind: 'unavailable', reason: 'shv' };
  }
  const choices = qualities(capabilities, transcoding, format, options);
  if (requested !== undefined && choices.includes(requested)) {
    if (requested === original) {
      return { kind: 'convert' };
    }

    if (requested === forked) {
      return { kind: 'convert', engine: 'fork' };
    }

    return requested === decoded ? { kind: 'decode' } : { kind: 'kyusu', preset: requested };
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
  // ブラウザの中での変換とデコードは kyusu の画質でないので、kyusu の画質だけから選ぶ。デコードする経路は、選ばれたときにだけ使う。
  const presets = choices.filter(
    (choice) => choice !== original && choice !== decoded && choice !== forked,
  );
  const preferred = format === 'UHD' && capabilities.convert ? uhdDesktopPreset : defaultPreset;
  const preset = presets.includes(preferred) ? preferred : presets[0];
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

// DataBroadcastSource は、データ放送の TS を流す配信。serviceId は、TS の program_number で、null なら PAT の最初の番組を選ぶ。
// elapsed を渡したら、録画型の再生元の配信として、読み始めた位置からの放送時刻の経過 (秒) に届くまで、TS を web-bml に渡さない (#2270)。
export type DataBroadcastSource = {
  url: string;
  serviceId: number | null;
  elapsed?: () => number | null;
};

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

// streamPath は、tenban のライブ視聴の TS (GET /internal/streams/services/{id}) の URL。
const streamPath = /^\/internal\/streams\/services\/(\d+)(?:\?(.*))?$/;

// blobPath は、blob: の URL のオリジンの後ろ。UUID だけを受け付ける。読み直すために付けた fragment は外す。
const blobPath = /^([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})(?:#\d*)?$/;

// recordingStreamPath は、録画の TS (GET /internal/streams/recordings/{id}) の URL。id は UUID。
const recordingStreamPath =
  /^\/internal\/streams\/recordings\/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})(?:\?(.*))?$/;

// decodeStreamEndpoint は、url がライブ視聴か録画の TS か、同じオリジンの blob: (端末に保存した録画) なら、
// 読みに行く URL を返す。ほかの先なら null を返す。origin は、このページのオリジン。
// ブラウザの中でデコードする経路の worker は、受け取った URL をそのまま読まずに、決まったパスへ組み直し、
// ほかのホストやパスを読まないようにする。録画は、シークした位置から読み直すので、start=秒 を付けたまま読む (#2166)。
export function decodeStreamEndpoint(
  url: string,
  origin: string | null = globalThis.location?.origin ?? null,
): string | null {
  // 端末に保存した録画は、ページが作った blob: の URL で渡す (#2166)。ほかのオリジンの blob: は読まない。
  // 受け取った URL はそのまま読まず、このページのオリジンと、URL の中の UUID から組み直す。
  if (origin !== null && origin !== 'null' && url.startsWith(`blob:${origin}/`)) {
    const blob = blobPath.exec(url.slice(`blob:${origin}/`.length));
    if (blob === null) {
      return null;
    }
    const [, id = ''] = blob;

    return `blob:${origin}/${encodeURIComponent(id)}`;
  }
  // 静的なデモ (#832) では、どの局もデモの映像を流すので、その URL も読む (#2157)。
  // 条件は import.meta.env.MODE を直に書き、通常のビルドでは定数の比較として消す。
  if (import.meta.env.MODE === 'demo') {
    const separator = url.indexOf('?');
    if ((separator === -1 ? url : url.slice(0, separator)) === demoStreamURL) {
      return separator === -1 ? demoStreamURL : `${demoStreamURL}${url.slice(separator)}`;
    }
  }
  const live = streamPath.exec(url);
  if (live !== null) {
    const [, id = '', query] = live;
    const path = `/internal/streams/services/${encodeURIComponent(id)}`;

    return query === undefined ? path : `${path}?${query}`;
  }
  const recording = recordingStreamPath.exec(url);
  if (recording !== null) {
    const [, id = '', query] = recording;
    const path = `/internal/streams/recordings/${encodeURIComponent(id)}`;
    // 問い合わせはそのまま渡さず、読み始める位置 (start) だけを組み直す。
    const start = Number(
      new URLSearchParams(query?.split('#', 1)[0] ?? '').get('start') ?? Number.NaN,
    );

    return Number.isFinite(start) && start >= 0 ? `${path}?start=${start.toFixed(1)}` : path;
  }

  return null;
}

// dataBroadcastPath は、tenban のデータ放送用のエンドポイント (GET /internal/streams/services/{id}/data-broadcast) の URL。
const dataBroadcastPath = /^\/internal\/streams\/services\/(\d+)\/data-broadcast(?:\?(.*))?$/;

// vodDataBroadcastPath は、録画型の再生元のデータ放送用のエンドポイント
// (GET /internal/streams/{recordings|timeshift|library}/{id}/data-broadcast、#2270) の URL。
const vodDataBroadcastPath =
  /^\/internal\/streams\/(recordings|timeshift|library)\/([A-Za-z0-9_-]+)\/data-broadcast(?:\?(.*))?$/;

// vodDataBroadcastURL は、録画型の再生元の映像の配信 streamUrl から、start 秒の位置から読むデータ放送の配信の URL を作る。
export function vodDataBroadcastURL(streamUrl: string, start: number): string {
  const [path = '', query] = streamUrl.split('?', 2);
  const params = new URLSearchParams(query);
  params.set('start', start.toFixed(1));

  return `${path}/data-broadcast?${params.toString()}`;
}

// vodStreamBases は、録画型の再生元の配信の種類。
const vodStreamBases = ['recordings', 'timeshift', 'library'] as const;

// rfc3339 は、タイムシフトのバッファの番組の始まり (from) に使う日時の文字。
const rfc3339 = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/;

// vodDataBroadcastQuery は、録画型の再生元のデータ放送の配信に渡す問い合わせを、読み始める位置 (start) と、
// タイムシフトのバッファの番組の始まり (from) だけで組み直す。受け取った問い合わせは、そのまま渡さない。
function vodDataBroadcastQuery(query: string | undefined): string {
  const given = new URLSearchParams(query?.split('#', 1)[0] ?? '');
  const params = new URLSearchParams();
  // 番組の始まりは、受け取った文字列を渡さず、日時として読んでから書き直す。
  const from = given.get('from');
  const fromTime = from !== null && rfc3339.test(from) ? Date.parse(from) : Number.NaN;
  if (Number.isFinite(fromTime)) {
    params.set('from', new Date(fromTime).toISOString());
  }
  const start = Number(given.get('start') ?? Number.NaN);
  if (Number.isFinite(start) && start >= 0) {
    params.set('start', start.toFixed(1));
  }

  return params.toString();
}

// dataBroadcastEndpoint は、url が tenban のデータ放送用のエンドポイントなら、読みに行く URL を返す。ほかの先なら null を返す。
// データ放送の Web Worker は、受け取った URL をそのまま読まずに、決まったパスへ組み直し、ほかのホストやパスを読まないようにする。
export function dataBroadcastEndpoint(url: string): string | null {
  const vod = vodDataBroadcastPath.exec(url);
  if (vod !== null) {
    const [, kind = '', id = '', query] = vod;
    // 種類は、受け取った文字列を URL に入れず、決まった名前から選ぶ。
    const base = vodStreamBases.find((candidate) => candidate === kind);
    if (base === undefined) {
      return null;
    }
    const path = `/internal/streams/${base}/${encodeURIComponent(id)}/data-broadcast`;
    const params = vodDataBroadcastQuery(query);

    return params === '' ? path : `${path}?${params}`;
  }
  const match = dataBroadcastPath.exec(url);
  if (match === null) {
    return null;
  }
  const [, id = '', query] = match;
  const path = `/internal/streams/services/${encodeURIComponent(id)}/data-broadcast`;

  return query === undefined ? path : `${path}?${query}`;
}
