// 静的なデモ (#832) の Service Worker。ライブ視聴と録画の再生の映像を、デモの映像 (demo/stream.m2ts) を繰り返して流す。
// 繰り返すたびに TS の時刻を進め (demo/stream-loop.js)、プレイヤーからは、途切れずに続く 1 本の放送や録画に見せる。
// ライブ視聴の要求 (?session= の付いたもの) と、録画の再生の要求 (?start= の付いたもの) だけを受け持ち、
// ほかの要求 (端末への保存やダウンロード) には手を出さない。それらは、ファイルをそのまま読む。
import { clockHz, indexStream, packetSize, StreamLooper } from './demo/stream-loop.js';

// period は、デモの映像の長さ (映像の 1920 フレーム、90 kHz の刻み)。scripts/build-demo-stream.sh で作る長さ。
const period = 1920 * 3003;
// leadSeconds は、ライブ視聴で、初めに一気に渡す分。プレイヤーが再生を始めるのに足りるだけ先に渡す。
const leadSeconds = 2;
// chunkPackets は、1 回に渡すパケットの数。
const chunkPackets = 64;

self.addEventListener('install', () => {
  void self.skipWaiting();
});

self.addEventListener('activate', (event) => {
  // 登録した直後のページも受け持ち、読み直さずに再生できるようにする。
  event.waitUntil(self.clients.claim());
});

// parameter は、URL の問い合わせから値を読む。録画の streamUrl には長さ (?length=) を付けてあり、
// プレイヤーはその後ろに ?start= を足すので、? が 2 つ並ぶ。URLSearchParams では読めないので、文字列から探す。
function parameter(search, name) {
  const match = new RegExp(`[?&]${name}=([0-9.]+)`).exec(search);

  return match === null ? null : Number(match[1]);
}

self.addEventListener('fetch', (event) => {
  const url = new URL(event.request.url);
  if (!url.pathname.endsWith('/demo/stream.m2ts')) {
    return;
  }
  const live = url.searchParams.has('session');
  const start = parameter(url.search, 'start');
  if (!live && start === null) {
    return;
  }
  url.search = '';
  if (live) {
    event.respondWith(liveStream(url.href));

    return;
  }
  const length = parameter(event.request.url, 'length');
  event.respondWith(recordingStream(url.href, event.request.method, start, length));
});

// loadIndex は、デモの映像を読み、繰り返すのに要る位置を集める。1 度だけ読み、使い回す。
let indexPromise = null;
function loadIndex(href) {
  indexPromise ??= fetch(href)
    .then((response) => {
      if (!response.ok) {
        throw new Error(`failed to fetch the demo stream: ${response.status}`);
      }

      return response.arrayBuffer();
    })
    .then((buffer) => indexStream(new Uint8Array(buffer), period))
    .catch((error) => {
      // 読めなければ、次の要求で読み直す。
      indexPromise = null;
      throw error;
    });

  return indexPromise;
}

// liveStream は、ライブ視聴の映像を、頭から実時間の速さで、終わらずに流す。
// 静的なファイルは一気に届き終わるので、そのまま渡すと、ライブ視聴の画面は再生を始める前に切れたものとして扱う。
async function liveStream(href) {
  let index;
  try {
    index = await loadIndex(href);
  } catch {
    return new Response(null, { status: 502 });
  }
  const looper = new StreamLooper(index);
  const startedAt = Date.now();
  // cancelled は、プレイヤーが読むのをやめたかどうか。待っている間にやめていれば、渡さずに終える。
  let cancelled = false;
  const body = new ReadableStream({
    async pull(controller) {
      // 次のパケットを渡してよい時刻まで待つ。初めの leadSeconds 分は待たずに渡す。
      const dueAt = startedAt + (looper.position / clockHz - leadSeconds) * 1000;
      const wait = dueAt - Date.now();
      if (wait > 0) {
        await new Promise((resolve) => setTimeout(resolve, wait));
      }
      if (cancelled) {
        return;
      }
      controller.enqueue(looper.read(chunkPackets));
    },
    cancel() {
      cancelled = true;
    },
  });

  return new Response(body, { headers: { 'Content-Type': 'video/mp2t' } });
}

// recordingStream は、録画の再生の映像を、start 秒の手前の I フレームから、length 秒まで流す。
// 速さは決めず、プレイヤーが読む分だけ渡す (プレイヤーは、再生の位置より先を変換しすぎないよう、読むのを止める)。
// 実際に流し始める位置は、tenban と同じく X-Kotatsu-Start で返す。HEAD でも返す。
async function recordingStream(href, method, start, length) {
  let index;
  try {
    index = await loadIndex(href);
  } catch {
    return new Response(null, { status: 502 });
  }
  const looper = new StreamLooper(index, {
    start: start * clockHz,
    end: length === null ? null : length * clockHz,
  });
  const headers = {
    'Content-Type': 'video/mp2t',
    'X-Kotatsu-Start': (looper.started / clockHz).toFixed(3),
  };
  if (method === 'HEAD') {
    return new Response(null, { headers });
  }
  const body = new ReadableStream(
    {
      pull(controller) {
        const chunk = looper.read(chunkPackets);
        if (chunk === null) {
          controller.close();

          return;
        }
        controller.enqueue(chunk);
      },
    },
    // 先に用意しておく分を、1 回に渡す分に抑える。プレイヤーが読むのを止めたら、こちらも作るのを止める。
    { highWaterMark: chunkPackets * packetSize, size: (chunk) => chunk.byteLength },
  );

  return new Response(body, { headers });
}
