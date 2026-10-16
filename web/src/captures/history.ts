// キャプチャの履歴 (#457、設計書 7.5)。撮った画像を、番組名、サービス名、放送時刻と一緒に、端末の IndexedDB に残す。
// どの番組のどこで撮ったかを、あとから探せるようにする。画像は大きいので、アカウントには保存しない。

// maxCaptures は、残すキャプチャの数の上限。超えたら、古いものから消す。1 枚は数百 KB から 2 MB ほど。
export const maxCaptures = 100;

// CaptureEntry は、残したキャプチャ。takenAt は撮った時刻、time は撮った場面の放送時刻 (どちらも Unix ミリ秒)。
// recordingId は、録画で撮ったときの録画の ID。ライブなら null。
export type CaptureEntry = {
  id: string;
  takenAt: number;
  time: number;
  serviceName: string;
  programName: string | null;
  recordingId: string | null;
  image: Blob;
};

// entriesToPrune は、上限を超えた分として消す、古いキャプチャの ID を返す。
export function entriesToPrune(
  entries: readonly Pick<CaptureEntry, 'id' | 'takenAt'>[],
  max = maxCaptures,
): string[] {
  if (entries.length <= max) {
    return [];
  }

  return [...entries]
    .sort((a, b) => a.takenAt - b.takenAt)
    .slice(0, entries.length - max)
    .map((entry) => entry.id);
}

// normalize は、探すための文字の形にそろえる。全角と半角、大文字と小文字を区別しない。
function normalize(text: string): string {
  return text.normalize('NFKC').toLowerCase();
}

// matchesQuery は、キャプチャが、探す言葉 (空白で区切ると、すべてを含むもの) に合うかを返す。番組名とサービス名から探す。
export function matchesQuery(
  entry: Pick<CaptureEntry, 'serviceName' | 'programName'>,
  query: string,
): boolean {
  const words = normalize(query)
    .split(/\s+/)
    .filter((word) => word !== '');
  const haystack = normalize(`${entry.programName ?? ''} ${entry.serviceName}`);

  return words.every((word) => haystack.includes(word));
}

const databaseName = 'kotatsu-captures';
const storeName = 'captures';

function request<T>(req: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    req.addEventListener('success', () => resolve(req.result));
    req.addEventListener('error', () => reject(req.error ?? new Error('IndexedDB request failed')));
  });
}

function openDatabase(): Promise<IDBDatabase> {
  const req = indexedDB.open(databaseName, 1);
  req.addEventListener('upgradeneeded', () => {
    req.result.createObjectStore(storeName, { keyPath: 'id' });
  });

  return request(req);
}

async function withStore<T>(
  mode: IDBTransactionMode,
  run: (store: IDBObjectStore) => Promise<T>,
): Promise<T> {
  const db = await openDatabase();
  try {
    return await run(db.transaction(storeName, mode).objectStore(storeName));
  } finally {
    db.close();
  }
}

// listCaptures は、残したキャプチャを、新しい順に返す。
export async function listCaptures(): Promise<CaptureEntry[]> {
  const entries = await withStore('readonly', (store) =>
    request(store.getAll() as IDBRequest<CaptureEntry[]>),
  );

  return entries.sort((a, b) => b.takenAt - a.takenAt);
}

// addCapture は、キャプチャを残し、上限を超えた分を古いものから消す。
export async function addCapture(entry: Omit<CaptureEntry, 'id' | 'takenAt'>): Promise<void> {
  const record: CaptureEntry = { ...entry, id: crypto.randomUUID(), takenAt: Date.now() };
  await withStore('readwrite', async (store) => {
    await request(store.put(record));
    const entries = await request(store.getAll() as IDBRequest<CaptureEntry[]>);
    await Promise.all(entriesToPrune(entries).map((id) => request(store.delete(id))));
  });
}

// deleteCapture は、キャプチャを消す。
export async function deleteCapture(id: string): Promise<void> {
  await withStore('readwrite', (store) => request(store.delete(id)));
}
