// demoStreamURL は、デモの映像 (64 秒の合成の映像) の URL。ライブ視聴と録画の再生で、どの局と録画もこれを流す。
// ブラウザの中でデコードする経路の worker も読むので、デモのデータとは別のファイルに置き、worker にデモのデータを含めない (#2157)。
export const demoStreamURL = `${import.meta.env.BASE_URL}demo/stream.m2ts`;
