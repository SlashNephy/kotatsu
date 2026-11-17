// ライブラリ (録画として登録していない TS のフォルダ) のファイラーと再生の画面で使う計算 (設計書 5.8)。

import { resumePosition } from '../yunomi/resume';

// LibrarySearch は、ファイラーと再生の画面の URL のクエリ。library はライブラリの名前、path はライブラリの中の / 区切りのパス。
// ファイラーでは、library を省くとライブラリの一覧を、path を省くとライブラリの根元を出す。
export type LibrarySearch = { library?: string; path?: string };

// parseLibrarySearch は、URL のクエリを読む。文字列でない値は捨てる。
export function parseLibrarySearch(search: Record<string, unknown>): LibrarySearch {
  const library =
    typeof search.library === 'string' && search.library !== '' ? search.library : undefined;
  const path = typeof search.path === 'string' && search.path !== '' ? search.path : undefined;

  return {
    ...(library === undefined ? {} : { library }),
    ...(path === undefined ? {} : { path }),
  };
}

// Crumb は、パンくずの 1 段。path は、そのディレクトリのライブラリの中のパス (根元は空)。
export type Crumb = { name: string; path: string };

// breadcrumbs は、ディレクトリ path (根元は空) の、根元から自分までのパンくずを返す。根元の名前はライブラリの名前にする。
// names は、サーバーが返した段ごとの名前 (LibraryDirectory.names)。Google Drive ではパスの段が ID なので、名前はこれで出す。
// 足りなければ、段をそのまま名前にする。
export function breadcrumbs(library: string, path: string, names: readonly string[] = []): Crumb[] {
  const crumbs: Crumb[] = [{ name: library, path: '' }];
  const parts = path === '' ? [] : path.split('/');
  parts.forEach((part, i) => {
    crumbs.push({ name: names[i] ?? part, path: parts.slice(0, i + 1).join('/') });
  });

  return crumbs;
}

// isMyDrive は、Google Drive のライブラリの、マイドライブの根元 (パス root) かどうかを返す。画面は、名前を訳して出す。
export function isMyDrive(kind: string, path: string): boolean {
  return kind === 'GOOGLE_DRIVE' && path === 'root';
}

// parentPath は、パス path の親のディレクトリのパスを返す。根元の中なら空。
export function parentPath(path: string): string {
  const at = path.lastIndexOf('/');

  return at < 0 ? '' : path.slice(0, at);
}

// FileIndex は、ファイルのシークの索引の進み具合 (LibraryFile.index)。
export type FileIndex = {
  complete: boolean;
  seekableSeconds: number;
  durationSeconds: number | null | undefined;
};

// fileDuration は、シークバーに出す長さ (秒) を返す。索引がまだなければ、長さが分からないので 0。
export function fileDuration(index: FileIndex): number {
  return index.durationSeconds ?? 0;
}

// seekLimit は、シークできる所 (秒) を返す。索引を作り終えていれば長さまで、作っている途中なら、索引のできた最後の I フレームまで。
// 配信は、求めた位置の手前の I フレームから流すので、索引のできた所より先を求めると、そこまで変換せずに読み進めることになる。
export function seekLimit(index: FileIndex): number {
  return index.complete ? fileDuration(index) : index.seekableSeconds;
}

// clampSeek は、シークの先 target を、0 からシークできる所までに収める。
export function clampSeek(target: number, index: FileIndex): number {
  return Math.min(Math.max(target, 0), seekLimit(index));
}

// libraryResumePosition は、保存した再生位置 savedMs (ミリ秒) から、再生を始める位置 (秒) を返す。録画と同じく少し戻して始め、
// 終わりの近くまで見ていたら最初から再生する。索引がまだ届いていなければ、届いた所までにする。
export function libraryResumePosition(savedMs: number | null, index: FileIndex): number {
  const duration = fileDuration(index);
  if (duration <= 0) {
    return 0;
  }

  return clampSeek(resumePosition(savedMs, duration), index);
}
