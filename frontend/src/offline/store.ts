// 録画の端末への保存 (#480、設計書 7.8)。保存先は、オリジンのプライベートファイルシステム (OPFS) にする。
// 何 GB にもなる TS を、メモリに溜めずに、届いた順に書けるからである。CacheStorage は、応答を丸ごと 1 つとして持つので、
// 書いている途中の進み具合を見せにくく、Range で途中から読ませるのも、応答を作り直さないとできない。
//
// 保存した録画は、録画ごとのディレクトリ (offline-recordings/<録画の ID>/) に、次のファイルで置く。
//   - video.m2ts: 録画の TS。配信 (設計書 7.6) が返す 1 本のファイルを、そのまま書く。
//   - jikkyo.json: 実況の過去ログ (設計書 7.4)。取れたときだけ置く。
//   - meta.json: 一覧と再生の画面に出す情報 (OfflineMeta)。ほかを書き終えてから最後に書く。
// meta.json のないディレクトリは、書きかけで止まった保存なので、一覧を読むときに消す。

import { useSyncExternalStore } from 'react';

import { timeZone } from '../futon/timeZone';
import { broadcastFontUrl } from '../yunomi/broadcastFont';
import type { Gap } from '../yunomi/position';
import { expectedSize, hasRoom, parseMeta, type OfflineMeta } from './offline';

const rootName = 'offline-recordings';
const videoName = 'video.m2ts';
const jikkyoName = 'jikkyo.json';
const metaName = 'meta.json';

// progressIntervalMs は、保存の進み具合を画面に知らせる間隔。届くたびに描き直すと重いので、間引く。
const progressIntervalMs = 250;

// SaveSource は、保存する録画。録画の詳細の画面が、GraphQL で読んだ値を渡す。
export type SaveSource = {
  id: string;
  name: string;
  serviceName: string;
  startAt: string;
  endAt: string;
  recordedFrom: string;
  recordedUntil: string;
  gaps: readonly Gap[];
  streamUrl: string;
  // jikkyoCommentsUrl は、実況の過去ログの URL。実況のチャンネルがなければ null。
  jikkyoCommentsUrl: string | null;
  files: readonly { kind: string; size: number | string }[];
};

// SaveError は、保存できなかった理由。quota は端末の空きが足りない、busy はほかのタブが同じ録画を保存している、
// failed はそのほか (回線が切れたなど)。
export type SaveError = 'quota' | 'busy' | 'failed';

// SaveState は、録画ごとの保存の状態。保存し終えたら、一覧 (saved) に移り、ここからは消える。
export type SaveState =
  | { kind: 'saving'; received: number; total: number | null }
  | { kind: 'error'; reason: SaveError };

type Snapshot = {
  // saved は、保存し終えた録画。読み込む前は null。
  saved: readonly OfflineMeta[] | null;
  states: ReadonlyMap<string, SaveState>;
  // persisted は、保存を永続化できたかどうか (navigator.storage.persist)。分からなければ null。
  persisted: boolean | null;
};

let snapshot: Snapshot = { saved: null, states: new Map(), persisted: null };
const listeners = new Set<() => void>();
// controllers は、このタブで保存している録画の中止に使う AbortController。
const controllers = new Map<string, AbortController>();

// lockName は、録画を保存している間に持つ Web Lock の名前。ほかのタブが、保存している途中のディレクトリを
// 書きかけの残りとして消したり、同じ録画を重ねて保存したりしないよう、タブをまたいで知らせる。
const lockName = (id: string) => `kotatsu-offline-recording:${id}`;

// savingElsewhere は、ほかのタブかこのタブが、録画 id を保存している途中かどうかを返す。
// Web Locks のないブラウザでは、このタブの保存だけを見る。
async function savingElsewhere(id: string): Promise<boolean> {
  if (controllers.has(id)) {
    return true;
  }
  if (navigator.locks === undefined) {
    return false;
  }

  return navigator.locks.request(lockName(id), { ifAvailable: true }, (lock) => lock === null);
}

function publish(next: Partial<Snapshot>) {
  snapshot = { ...snapshot, ...next };
  for (const listener of listeners) {
    listener();
  }
}

function setState(id: string, state: SaveState | null) {
  const states = new Map(snapshot.states);
  if (state === null) {
    states.delete(id);
  } else {
    states.set(id, state);
  }
  publish({ states });
}

async function rootDirectory(): Promise<FileSystemDirectoryHandle> {
  const root = await navigator.storage.getDirectory();

  return root.getDirectoryHandle(rootName, { create: true });
}

async function readText(directory: FileSystemDirectoryHandle, name: string): Promise<string> {
  const handle = await directory.getFileHandle(name);

  return (await handle.getFile()).text();
}

async function writeText(directory: FileSystemDirectoryHandle, name: string, text: string) {
  const handle = await directory.getFileHandle(name, { create: true });
  const writable = await handle.createWritable();
  await writable.write(text);
  await writable.close();
}

// removeDirectory は、録画のディレクトリを消す。もうなければ何もしない。
async function removeDirectory(root: FileSystemDirectoryHandle, id: string) {
  try {
    await root.removeEntry(id, { recursive: true });
  } catch (error) {
    if (!(error instanceof DOMException && error.name === 'NotFoundError')) {
      throw error;
    }
  }
}

// refreshOfflineRecordings は、保存した録画の一覧を読み直す。書きかけで止まった保存は、ここで消す。
export async function refreshOfflineRecordings(): Promise<void> {
  const root = await rootDirectory();
  const saved: OfflineMeta[] = [];
  const leftovers: string[] = [];
  for await (const [name, handle] of root.entries()) {
    if (handle.kind !== 'directory') {
      continue;
    }
    let meta: OfflineMeta | null = null;
    try {
      meta = parseMeta(await readText(handle, metaName));
    } catch {
      meta = null;
    }
    if (meta === null) {
      // 今保存している録画は、まだ meta.json がないだけなので、消さない。
      if (!(await savingElsewhere(name))) {
        leftovers.push(name);
      }
      continue;
    }
    saved.push(meta);
  }
  // 消せなくても (ほかのタブが開いているなど)、一覧は出す。次に読み直すときに、また消してみる。
  for (const id of leftovers) {
    await removeDirectory(root, id).catch(() => {});
  }
  // 新しく放送したものから並べる。
  saved.sort((a, b) => Date.parse(b.startAt) - Date.parse(a.startAt));
  const persisted = (await navigator.storage.persisted?.()) ?? null;
  publish({ saved, persisted });
}

// saveRecording は、録画を端末に保存する。保存している間の進み具合は、useOfflineRecordings で読む。
export async function saveRecording(source: SaveSource): Promise<void> {
  if (controllers.has(source.id)) {
    return;
  }
  const controller = new AbortController();
  controllers.set(source.id, controller);
  setState(source.id, { kind: 'saving', received: 0, total: null });
  if (navigator.locks === undefined) {
    await writeRecording(source, controller);
  } else {
    // ほかのタブが同じ録画を保存している間は、重ねて保存しない。
    const started = await navigator.locks.request(
      lockName(source.id),
      { ifAvailable: true },
      async (lock) => {
        if (lock === null) {
          return false;
        }
        await writeRecording(source, controller);

        return true;
      },
    );
    if (!started) {
      controllers.delete(source.id);
      setState(source.id, { kind: 'error', reason: 'busy' });
    }
  }
  await refreshOfflineRecordings().catch(() => {});
  announceChange();
}

// writeRecording は、録画を保存する。保存し終えるか、止めるか、失敗するまで待つ。
async function writeRecording(source: SaveSource, controller: AbortController): Promise<void> {
  let root: FileSystemDirectoryHandle | null = null;
  try {
    root = await rootDirectory();
    // 前に保存したものが残っていれば、消してから書き直す。
    await removeDirectory(root, source.id);
    const directory = await root.getDirectoryHandle(source.id, { create: true });
    // 録画を終えた録画は、引き継ぎで重なった区間を切り詰めた 1 本のファイルとして返る (設計書 7.6)。
    // ブラウザの HTTP のキャッシュにも残すと、端末の容量を二重に使うので、キャッシュさせない。
    const response = await fetch(source.streamUrl, {
      cache: 'no-store',
      credentials: 'same-origin',
      signal: controller.signal,
    });
    if (!response.ok || response.body === null) {
      throw new Error(`unexpected status ${response.status}`);
    }
    const total = expectedSize(response.headers.get('Content-Length'), source.files);
    // 書き始める前に、端末の空きを確かめ、ブラウザに消されないよう、保存の永続化を求める。
    // 永続化を断られても保存はできるので、断られたことは一覧に出すだけにする。
    if (!hasRoom(await navigator.storage.estimate(), total)) {
      await response.body.cancel();
      throw new DOMException('not enough storage', 'QuotaExceededError');
    }
    const persisted = (await navigator.storage.persist?.()) ?? null;
    publish({ persisted });
    setState(source.id, { kind: 'saving', received: 0, total });

    let received = 0;
    let reported = 0;
    const counter = new TransformStream<Uint8Array, Uint8Array>({
      transform(chunk, sink) {
        received += chunk.byteLength;
        const now = Date.now();
        if (now - reported >= progressIntervalMs) {
          reported = now;
          setState(source.id, { kind: 'saving', received, total });
        }
        sink.enqueue(chunk);
      },
    });
    const video = await directory.getFileHandle(videoName, { create: true });
    await response.body
      .pipeThrough(counter)
      .pipeTo(await video.createWritable(), { signal: controller.signal });

    // 実況の過去ログは、取れなくても映像は見られるので、保存は続ける。
    let jikkyo = false;
    if (source.jikkyoCommentsUrl !== null) {
      try {
        const comments = await fetch(source.jikkyoCommentsUrl, {
          credentials: 'same-origin',
          signal: controller.signal,
        });
        if (comments.ok) {
          await writeText(directory, jikkyoName, await comments.text());
          jikkyo = true;
        }
      } catch (error) {
        if (controller.signal.aborted) {
          throw error;
        }
      }
    }
    // 字幕を描くフォントは大きいので、アプリの外枠と一緒にはキャッシュしない。Service Worker は、読んだフォントを
    // キャッシュに残すので (src/sw.ts)、オフラインでも字幕を描けるよう、ここで一度読んでおく。
    void fetch(broadcastFontUrl).catch(() => {});

    const meta: OfflineMeta = {
      version: 1,
      id: source.id,
      name: source.name,
      serviceName: source.serviceName,
      startAt: source.startAt,
      endAt: source.endAt,
      recordedFrom: source.recordedFrom,
      recordedUntil: source.recordedUntil,
      gaps: [...source.gaps],
      size: received,
      jikkyo,
      timeZone: timeZone(),
      savedAt: new Date().toISOString(),
    };
    await writeText(directory, metaName, JSON.stringify(meta));
    controllers.delete(source.id);
    setState(source.id, null);
  } catch (error) {
    controllers.delete(source.id);
    // 書きかけのファイルは、残しても再生できないので消す。
    if (root !== null) {
      await removeDirectory(root, source.id).catch(() => {});
    }
    if (controller.signal.aborted) {
      setState(source.id, null);
    } else {
      setState(source.id, {
        kind: 'error',
        reason:
          error instanceof DOMException && error.name === 'QuotaExceededError' ? 'quota' : 'failed',
      });
    }
  }
}

// cancelSave は、保存を止める。書きかけのファイルは消す。
export function cancelSave(id: string): void {
  controllers.get(id)?.abort();
}

// dismissSaveError は、保存できなかった知らせを消す。
export function dismissSaveError(id: string): void {
  if (snapshot.states.get(id)?.kind === 'error') {
    setState(id, null);
  }
}

// removeOfflineRecording は、端末に保存した録画を消す。サーバーの録画は消さない。
export async function removeOfflineRecording(id: string): Promise<void> {
  await removeDirectory(await rootDirectory(), id);
  await refreshOfflineRecordings();
  announceChange();
}

// OfflineFiles は、保存した録画を再生するためのファイル。
export type OfflineFiles = { meta: OfflineMeta; video: File; jikkyo: File | null };

// openOfflineRecording は、保存した録画のファイルを開く。保存していなければ null を返す。
// OPFS の File は、中身をメモリに読まずに、ディスクから読む。
export async function openOfflineRecording(id: string): Promise<OfflineFiles | null> {
  const root = await rootDirectory();
  let directory: FileSystemDirectoryHandle;
  try {
    directory = await root.getDirectoryHandle(id);
  } catch {
    return null;
  }
  try {
    const meta = parseMeta(await readText(directory, metaName));
    if (meta === null) {
      return null;
    }
    const video = await (await directory.getFileHandle(videoName)).getFile();
    let jikkyo: File | null = null;
    if (meta.jikkyo) {
      jikkyo = await (await directory.getFileHandle(jikkyoName)).getFile();
    }

    return { meta, video, jikkyo };
  } catch {
    return null;
  }
}

// changes は、保存した録画が変わったことを、同じオリジンのほかのタブに知らせる BroadcastChannel。初めて読まれたときに作る。
let changes: BroadcastChannel | null = null;

// announceChange は、保存した録画が変わったことを、ほかのタブに知らせる。
function announceChange() {
  changes?.postMessage('changed');
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  // 初めて読まれたときに、一覧を読み込み、ほかのタブで保存したり消したりしたら読み直す。
  if (snapshot.saved === null && listeners.size === 1) {
    void refreshOfflineRecordings().catch(() => publish({ saved: [] }));
    if (changes === null && typeof BroadcastChannel !== 'undefined') {
      changes = new BroadcastChannel('kotatsu-offline-recordings');
      changes.onmessage = () => void refreshOfflineRecordings().catch(() => {});
    }
  }

  return () => listeners.delete(listener);
}

// useOfflineRecordings は、保存した録画の一覧と、保存している録画の進み具合を返す。
// 画面を移っても保存は続くので、状態はモジュールに持つ。タブを閉じると、保存は止まる。
export function useOfflineRecordings(): Snapshot {
  return useSyncExternalStore(subscribe, () => snapshot);
}
