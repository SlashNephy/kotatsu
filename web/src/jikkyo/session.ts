// NX-Jikkyo のライブのコメントを受け取り、投稿する (設計書 7.4)。
// ニコニコ生放送の旧来の API と同じく、視聴セッションと、コメントセッションの 2 本の WebSocket を使う。

// nxJikkyoOrigin は、NX-Jikkyo の WebSocket の接続先。CSP で、この接続先だけを許している (設計書 10.4)。
export const nxJikkyoOrigin = 'wss://nx-jikkyo.tsukumijima.net';

// LiveComment は、届いたコメント。at は、受け取った時刻 (ミリ秒、performance.now の時計)。
export type LiveComment = { content: string; mail: string; at: number; userId?: string };

export type SessionStatus =
  | { kind: 'connecting' }
  | { kind: 'connected' }
  // disconnected は、切れたこと。reason は、NX-Jikkyo が返した理由のコード。retryIn 秒後につなぎ直す。
  | { kind: 'disconnected'; reason: string; retryIn: number };

export type SessionHandlers = {
  onComment: (comment: LiveComment) => void;
  onStatus: (status: SessionStatus) => void;
};

type SocketLike = Pick<WebSocket, 'send' | 'close' | 'addEventListener'>;

export type SessionOptions = {
  // createSocket と now と setTimer は、テストで差し替える。
  createSocket?: (url: string) => SocketLike;
  now?: () => number;
  setTimer?: (callback: () => void, ms: number) => () => void;
};

// retryDelays は、切れてからつなぎ直すまでの秒。続けて切れるほど延ばす。
const retryDelays = [3, 5, 10, 20, 30];

// commentSessionHandshake は、コメントセッションを開いたときに送るメッセージ。直近の 100 件を求め、rf:0 の ping で、
// 求めた分を送り終えたことを知らせてもらう。
export function commentSessionHandshake(threadId: string, threadKey: string): unknown[] {
  return [
    { ping: { content: 'rs:0' } },
    { ping: { content: 'ps:0' } },
    {
      thread: {
        version: '20061206',
        thread: threadId,
        threadkey: threadKey,
        user_id: '',
        res_from: -100,
      },
    },
    { ping: { content: 'pf:0' } },
    { ping: { content: 'rf:0' } },
  ];
}

// isNXJikkyoURL は、uri が NX-Jikkyo の WebSocket の接続先かどうかを返す。
function isNXJikkyoURL(uri: string): boolean {
  return uri.startsWith(`${nxJikkyoOrigin}/`);
}

// isPostError は、エラーのコードが、投稿の失敗を表すかどうかを返す。
function isPostError(code: string): boolean {
  return code.startsWith('COMMENT_') || code === 'INVALID_MESSAGE';
}

type Pending = { resolve: () => void; reject: (error: Error) => void };

// NXJikkyoSession は、実況のチャンネル channelId のコメントを受け取り続ける。切れたら、間隔を延ばしながらつなぎ直す。
export class NXJikkyoSession {
  private readonly channelId: string;
  private readonly handlers: SessionHandlers;
  private readonly createSocket: (url: string) => SocketLike;
  private readonly now: () => number;
  private readonly setTimer: (callback: () => void, ms: number) => () => void;
  private watch: SocketLike | null = null;
  private comment: SocketLike | null = null;
  private cancelTimers: (() => void)[] = [];
  // cancelKeepSeat は、座席を保つ keepSeat の繰り返しを止める。seat が届き直したら、前の繰り返しを止めてから始め直す。
  private cancelKeepSeat: (() => void) | null = null;
  private vposBaseTime = 0;
  private attempt = 0;
  private closed = false;
  // generation は、つなぎ直すたびに増やす。前の接続から遅れて届いたイベントを捨てる。
  private generation = 0;
  private pending: Pending | null = null;

  constructor(channelId: string, handlers: SessionHandlers, options: SessionOptions = {}) {
    this.channelId = channelId;
    this.handlers = handlers;
    this.createSocket = options.createSocket ?? ((url) => new WebSocket(url));
    this.now = options.now ?? (() => performance.now());
    this.setTimer =
      options.setTimer ??
      ((callback, ms) => {
        const id = setTimeout(callback, ms);

        return () => clearTimeout(id);
      });
    this.connect();
  }

  // post は、コメントを匿名で投稿する。視聴セッションがつながっていなければ失敗する。
  post(text: string): Promise<void> {
    const watch = this.watch;
    if (watch === null || this.comment === null) {
      return Promise.reject(new Error('not connected'));
    }
    this.pending?.reject(new Error('superseded'));

    return new Promise((resolve, reject) => {
      this.pending = { resolve, reject };
      watch.send(
        JSON.stringify({
          type: 'postComment',
          data: {
            text,
            color: 'white',
            position: 'naka',
            size: 'medium',
            // vpos は、番組の基準の時刻からの 1/100 秒。小数は拒まれる。
            vpos: Math.round((Date.now() - this.vposBaseTime) / 10),
            isAnonymous: true,
          },
        }),
      );
    });
  }

  close(): void {
    this.closed = true;
    this.teardown();
  }

  private connect(): void {
    const generation = ++this.generation;
    const current = () => generation === this.generation && !this.closed;
    this.handlers.onStatus({ kind: 'connecting' });
    const watch = this.createSocket(`${nxJikkyoOrigin}/api/v1/channels/${this.channelId}/ws/watch`);
    this.watch = watch;
    watch.addEventListener('open', () => {
      if (current()) {
        watch.send(JSON.stringify({ type: 'startWatching', data: { reconnect: false } }));
      }
    });
    watch.addEventListener('message', (event) => {
      if (current()) {
        this.onWatchMessage(JSON.parse(String(event.data)) as WatchMessage, generation);
      }
    });
    const lost = () => {
      if (current()) {
        this.reconnect('CONNECTION_CLOSED');
      }
    };
    watch.addEventListener('close', lost);
    watch.addEventListener('error', lost);
  }

  private onWatchMessage(message: WatchMessage, generation: number): void {
    switch (message.type) {
      case 'seat': {
        // 座席を保つため、言われた間隔で keepSeat を送る。
        const interval = (message.data?.keepIntervalSec ?? 30) * 1000;
        const keep = () => {
          this.watch?.send(JSON.stringify({ type: 'keepSeat' }));
          this.cancelKeepSeat = this.setTimer(keep, interval);
        };
        this.cancelKeepSeat?.();
        this.cancelKeepSeat = this.setTimer(keep, interval);
        break;
      }
      case 'ping':
        this.watch?.send(JSON.stringify({ type: 'pong' }));
        break;
      case 'room':
        this.vposBaseTime = Date.parse(message.data.vposBaseTime);
        this.openCommentSession(
          message.data.messageServer.uri,
          message.data.threadId,
          message.data.yourPostKey ?? '',
          generation,
        );
        break;
      case 'postCommentResult':
        this.pending?.resolve();
        this.pending = null;
        break;
      case 'error':
        // 投稿の失敗は、投稿を待っている側に返す。それ以外のエラーは、つなぎ直す。
        const code = message.data?.code;
        if (this.pending !== null && code !== undefined && isPostError(code)) {
          this.pending.reject(new Error(code));
          this.pending = null;
          break;
        }
        this.reconnect(code ?? 'ERROR');
        break;
      case 'disconnect':
        this.reconnect(message.data?.reason ?? 'DISCONNECTED');
        break;
      default:
        break;
    }
  }

  private openCommentSession(
    uri: string,
    threadId: string,
    threadKey: string,
    generation: number,
  ): void {
    // room が届き直したら (スレッドが変わったときなど)、前のコメントセッションを閉じてから開き直す。
    // 閉じた前のセッションから遅れて届いたイベントは、今のセッションのものでないので捨てる。
    const previous = this.comment;
    this.comment = null;
    previous?.close();
    // コメントセッションの接続先は、視聴セッションから届く。NX-Jikkyo 以外を指していたら、接続せずにつなぎ直す。
    // CSP でも止まるが、止まったことが分かるよう、ここで断る。
    if (!isNXJikkyoURL(uri)) {
      this.reconnect('INVALID_MESSAGE_SERVER');

      return;
    }
    const comment = this.createSocket(uri);
    this.comment = comment;
    const current = () =>
      generation === this.generation && !this.closed && this.comment === comment;
    // 最初に届く直近のコメントは、もう流れ終えたものなので流さない。rf:0 の ping で、届き終えたと分かる。
    let caughtUp = false;
    comment.addEventListener('open', () => {
      if (current()) {
        comment.send(JSON.stringify(commentSessionHandshake(threadId, threadKey)));
      }
    });
    comment.addEventListener('message', (event) => {
      if (!current()) {
        return;
      }
      const message = JSON.parse(String(event.data)) as CommentMessage;
      if (message.ping?.content === 'rf:0') {
        caughtUp = true;
        this.attempt = 0;
        this.handlers.onStatus({ kind: 'connected' });

        return;
      }
      const chat = message.chat;
      // 自分の投稿は、投稿した側で先に流すので、届いても流さない。
      if (!caughtUp || chat === undefined || chat.yourpost === 1) {
        return;
      }
      this.handlers.onComment({
        content: chat.content ?? '',
        mail: chat.mail ?? '',
        at: this.now(),
        ...(chat.user_id === undefined || chat.user_id === '' ? {} : { userId: chat.user_id }),
      });
    });
    const lost = () => {
      if (current()) {
        this.reconnect('CONNECTION_CLOSED');
      }
    };
    comment.addEventListener('close', lost);
    comment.addEventListener('error', lost);
  }

  private reconnect(reason: string): void {
    this.teardown();
    if (this.closed) {
      return;
    }
    const retryIn = retryDelays[Math.min(this.attempt, retryDelays.length - 1)] ?? 30;
    this.attempt += 1;
    this.handlers.onStatus({ kind: 'disconnected', reason, retryIn });
    this.cancelTimers.push(this.setTimer(() => this.connect(), retryIn * 1000));
  }

  private teardown(): void {
    this.generation += 1;
    for (const cancel of this.cancelTimers) {
      cancel();
    }
    this.cancelTimers = [];
    this.cancelKeepSeat?.();
    this.cancelKeepSeat = null;
    this.pending?.reject(new Error('disconnected'));
    this.pending = null;
    this.watch?.close();
    this.comment?.close();
    this.watch = null;
    this.comment = null;
  }
}

type WatchMessage =
  | { type: 'seat'; data?: { keepIntervalSec?: number } }
  | { type: 'ping' }
  | {
      type: 'room';
      data: {
        messageServer: { uri: string };
        threadId: string;
        yourPostKey?: string;
        vposBaseTime: string;
      };
    }
  | { type: 'postCommentResult' }
  | { type: 'error'; data?: { code?: string } }
  | { type: 'disconnect'; data?: { reason?: string } };

type CommentMessage = {
  ping?: { content: string };
  chat?: { content?: string; mail?: string; user_id?: string; yourpost?: number };
};
