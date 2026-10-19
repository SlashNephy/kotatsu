// 静的なデモ (#832) の Service Worker。ライブ視聴の映像を、実時間の速さで流す。
// 静的なファイルは一気に届き終わるので、そのまま渡すと、ライブ視聴の画面は再生を始める前に切れたものとして扱う。
// ライブ視聴の要求 (?session= の付いたもの) だけを受け持ち、ほかの要求には手を出さない。
// 映像が終わると切れたものとして扱われ、画面のつなぎ直しで、また初めから流れる。

// durationSeconds は、映像の長さ (web/scripts/build-demo-stream.sh で作る長さ)。
const durationSeconds = 20;
// leadSeconds は、初めに一気に渡す分。プレイヤーが再生を始めるのに足りるだけ先に渡す。
const leadSeconds = 2;
// chunkBytes は、1 回に渡す大きさ。TS のパケット (188 バイト) の倍数にする。
const chunkBytes = 188 * 64;

self.addEventListener('install', () => {
  void self.skipWaiting();
});

self.addEventListener('activate', (event) => {
  // 登録した直後のページも受け持ち、読み直さずにライブ視聴を流せるようにする。
  event.waitUntil(self.clients.claim());
});

self.addEventListener('fetch', (event) => {
  const url = new URL(event.request.url);
  if (!url.pathname.endsWith('/demo/stream.m2ts') || !url.searchParams.has('session')) {
    return;
  }
  url.search = '';
  event.respondWith(paced(url.href));
});

async function paced(href) {
  const response = await fetch(href);
  const bytes = new Uint8Array(await response.arrayBuffer());
  const bytesPerSecond = bytes.length / durationSeconds;
  const startedAt = Date.now();
  let offset = 0;

  const body = new ReadableStream({
    async pull(controller) {
      if (offset >= bytes.length) {
        controller.close();

        return;
      }
      // offset までを渡してよい時刻まで待つ。初めの leadSeconds 分は待たずに渡す。
      const dueAt = startedAt + (offset / bytesPerSecond - leadSeconds) * 1000;
      const wait = dueAt - Date.now();
      if (wait > 0) {
        await new Promise((resolve) => setTimeout(resolve, wait));
      }
      const end = Math.min(offset + chunkBytes, bytes.length);
      controller.enqueue(bytes.slice(offset, end));
      offset = end;
    },
  });

  return new Response(body, { headers: { 'Content-Type': 'video/mp2t' } });
}
