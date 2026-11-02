// LibraryStatus は、視聴管理の画面 (#1231) で絞り込む作品の状態。URL のクエリの status に持つ (設計書 10.3)。
export type LibraryStatus = 'WATCHING' | 'WANNA_WATCH' | 'all';

// libraryStatuses は、絞り込みの並び。既定は「見てる」。
export const libraryStatuses: readonly LibraryStatus[] = ['WATCHING', 'WANNA_WATCH', 'all'];

// parseLibraryStatus は、URL のクエリを、絞り込む状態にする。知らない値は既定 (見てる) にする。
export function parseLibraryStatus(value: unknown): LibraryStatus {
  return libraryStatuses.find((status) => status === value) ?? 'WATCHING';
}
