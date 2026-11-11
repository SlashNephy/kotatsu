import type { JikkyoComment } from '../jikkyo/comments';
import { jikkyoSocketPath } from '../jikkyo/session';

// 静的なデモ (#832、#1050) の実況。外部の NX-Jikkyo にはつながず、架空のコメントをブラウザの中で流す。
// ライブ視聴は、tenban の実況の中継の WebSocket を偽物に差し替え、録画の再生は、過去ログの応答を fetch の横取りで返す。
// コメントは、デモの映像 (demo/stream.m2ts、山のふもとの朝の景色) に合わせた、架空のもの。

// demoCommentsPath は、録画の再生で読む、実況の過去ログの URL。src/demo/index.ts が横取りして返す。
export const demoCommentsPath = '/demo/jikkyo';

const lines = [
  'おはよう',
  '空きれい',
  '霜すごいな',
  'こたつから出られない',
  'ぬくぬく',
  '電波塔見える',
  '寒そう',
  '晴れてよかった',
  'みかん食べたい',
  '山の雪きれい',
  'ふもと行ってみたい',
  'のどかだなあ',
  '8888',
  'きょうはぽかぽか',
  '雲ゆっくり',
  '洗濯日和',
  '家かわいい',
  'いい朝',
];

// mails は、コメントに付けるコマンド。ときどき色や位置を変える。
const mails = ['', '', '', 'yellow', '', 'shita', '', 'cyan'];

// demoComment は、n 番目の架空のコメントを返す。同じ n なら、同じコメントになる。
export function demoComment(n: number): Omit<JikkyoComment, 'time'> {
  return {
    content: lines[n % lines.length]!,
    mail: mails[n % mails.length]!,
    userId: `demo-user-${n % 7}`,
  };
}

// demoCommentGap は、n 番目のコメントから次のコメントまでの秒。1 秒前後で揺らす。
export function demoCommentGap(n: number): number {
  return 0.6 + ((n * 7) % 5) * 0.2;
}

// demoRecordedComments は、録画の再生で流す過去ログ。録画は、どれもデモの映像 (64 秒) を流すので、その間に並べる。
export function demoRecordedComments(seconds = 64): JikkyoComment[] {
  const comments: JikkyoComment[] = [];
  let time = 0.5;
  for (let n = 0; time < seconds; n++) {
    comments.push({ time: Math.round(time * 100) / 100, ...demoComment(n) });
    time += demoCommentGap(n);
  }

  return comments;
}

// FakeJikkyoSocket は、tenban の実況の中継の WebSocket の振りをする。
// src/jikkyo/session.ts が送り、受け取るメッセージだけを扱う。
export class FakeJikkyoSocket extends EventTarget {
  readonly url: string;
  private readonly timers = new Set<ReturnType<typeof setTimeout>>();
  private closed = false;
  private count = 0;

  constructor(url: string) {
    super();
    this.url = url;
    this.later(() => {
      this.dispatchEvent(new Event('open'));
      this.reply({ type: 'status', kind: 'connected' });
      this.flow();
    }, 0);
  }

  send(data: string): void {
    const message = JSON.parse(data) as { type?: string; id?: number };
    if (message.type === 'post') {
      // 投稿は、どこにも送らずに受け付けたことにする。自分の投稿は、画面が先に流す。
      this.reply({ type: 'postResult', id: message.id });
    }
  }

  close(): void {
    this.closed = true;
    for (const timer of this.timers) {
      clearTimeout(timer);
    }
    this.timers.clear();
  }

  // flow は、架空のコメントを、間を空けながら送り続ける。
  private flow(): void {
    const n = this.count++;
    this.later(
      () => {
        const comment = demoComment(n);
        this.reply({
          type: 'comment',
          content: comment.content,
          mail: comment.mail,
          userId: comment.userId,
        });
        this.flow();
      },
      demoCommentGap(n) * 1000,
    );
  }

  private reply(message: unknown): void {
    this.later(() => {
      this.dispatchEvent(new MessageEvent('message', { data: JSON.stringify(message) }));
    }, 0);
  }

  private later(callback: () => void, ms: number): void {
    const timer = setTimeout(() => {
      this.timers.delete(timer);
      if (!this.closed) {
        callback();
      }
    }, ms);
    this.timers.add(timer);
  }
}

// installFakeJikkyo は、実況の中継の WebSocket を、偽物に差し替える。ほかの接続先は、そのまま本物を使う。
export function installFakeJikkyo(): void {
  const Original = window.WebSocket;
  const Patched = function (url: string | URL, protocols?: string | string[]) {
    const href = String(url);
    if (new URL(href).pathname.startsWith(jikkyoSocketPath)) {
      return new FakeJikkyoSocket(href);
    }

    return new Original(url, protocols);
  } as unknown as typeof WebSocket;
  Object.assign(Patched, {
    CONNECTING: Original.CONNECTING,
    OPEN: Original.OPEN,
    CLOSING: Original.CLOSING,
    CLOSED: Original.CLOSED,
  });
  Patched.prototype = Original.prototype;
  window.WebSocket = Patched;
}
