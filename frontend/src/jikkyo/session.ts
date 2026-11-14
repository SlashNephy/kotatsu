// ライブのコメントを tenban から受け取り、投稿する (設計書 7.4)。
// NX-Jikkyo には tenban がつなぐ。視聴者の IP アドレスと見ているチャンネルを、第三者に渡さないためである。

// jikkyoSocketPath は、実況のコメントを中継する WebSocket のパスの前半。
export const jikkyoSocketPath = '/internal/media/jikkyo/';

// jikkyoSocketURL は、実況のチャンネル channelId のコメントを中継する WebSocket の URL を返す。同じオリジンにつなぐ。
export function jikkyoSocketURL(channelId: string, location: Location = window.location): string {
  const scheme = location.protocol === 'https:' ? 'wss:' : 'ws:';

  return `${scheme}//${location.host}${jikkyoSocketPath}${encodeURIComponent(channelId)}/ws`;
}

// LiveComment は、届いたコメント。at は、受け取った時刻 (ミリ秒、performance.now の時計)。
// postedAt は、書き込まれた時刻 (Unix ミリ秒、#1515)。tenban が知らせなければない。
export type LiveComment = {
  content: string;
  mail: string;
  at: number;
  userId?: string;
  postedAt?: number;
};

export type SessionStatus =
  | { kind: 'connecting' }
  | { kind: 'connected' }
  // disconnected は、切れたこと。reason は、切れた理由のコード。retryIn 秒後につなぎ直す。
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
  // url は、テストで差し替える。
  url?: string;
};

// retryDelays は、tenban との接続が切れてからつなぎ直すまでの秒。続けて切れるほど延ばす。
const retryDelays = [3, 5, 10, 20, 30];

type Pending = { id: number; resolve: () => void; reject: (error: Error) => void };

// ServerMessage は、tenban から届くメッセージ。
type ServerMessage =
  | { type: 'status'; kind: 'connecting' | 'connected' }
  | { type: 'status'; kind: 'disconnected'; reason?: string; retryIn?: number }
  | { type: 'comment'; content?: string; mail?: string; userId?: string; postedAt?: number }
  | { type: 'postResult'; id: number; error?: string };

// JikkyoSession は、実況のチャンネル channelId のコメントを受け取り続ける。切れたら、間隔を延ばしながらつなぎ直す。
export class JikkyoSession {
  private readonly url: string;
  private readonly handlers: SessionHandlers;
  private readonly createSocket: (url: string) => SocketLike;
  private readonly now: () => number;
  private readonly setTimer: (callback: () => void, ms: number) => () => void;
  private socket: SocketLike | null = null;
  private opened = false;
  private cancelRetry: (() => void) | null = null;
  private attempt = 0;
  private closed = false;
  private nextPostId = 1;
  private pending: Pending | null = null;

  constructor(channelId: string, handlers: SessionHandlers, options: SessionOptions = {}) {
    this.url = options.url ?? jikkyoSocketURL(channelId);
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

  // post は、コメントを匿名で投稿する。つながっていなければ失敗する。
  post(text: string): Promise<void> {
    const socket = this.socket;
    if (socket === null || !this.opened) {
      return Promise.reject(new Error('not connected'));
    }
    this.pending?.reject(new Error('superseded'));
    const id = this.nextPostId++;

    return new Promise((resolve, reject) => {
      this.pending = { id, resolve, reject };
      socket.send(JSON.stringify({ type: 'post', id, text }));
    });
  }

  close(): void {
    this.closed = true;
    this.teardown();
  }

  private connect(): void {
    this.handlers.onStatus({ kind: 'connecting' });
    const socket = this.createSocket(this.url);
    this.socket = socket;
    const current = () => socket === this.socket && !this.closed;
    socket.addEventListener('open', () => {
      if (current()) {
        this.opened = true;
      }
    });
    socket.addEventListener('message', (event) => {
      if (current()) {
        this.onMessage(JSON.parse(String(event.data)) as ServerMessage);
      }
    });
    const lost = () => {
      if (current()) {
        this.reconnect();
      }
    };
    socket.addEventListener('close', lost);
    socket.addEventListener('error', lost);
  }

  private onMessage(message: ServerMessage): void {
    switch (message.type) {
      case 'status':
        if (message.kind === 'disconnected') {
          // tenban と NX-Jikkyo の間が切れた。つなぎ直すのは tenban なので、知らせるだけにする。
          this.handlers.onStatus({
            kind: 'disconnected',
            reason: message.reason ?? 'DISCONNECTED',
            retryIn: message.retryIn ?? 0,
          });
          break;
        }
        if (message.kind === 'connected') {
          this.attempt = 0;
        }
        this.handlers.onStatus({ kind: message.kind });
        break;
      case 'comment':
        this.handlers.onComment({
          content: message.content ?? '',
          mail: message.mail ?? '',
          at: this.now(),
          ...(message.userId === undefined || message.userId === ''
            ? {}
            : { userId: message.userId }),
          ...(message.postedAt === undefined || message.postedAt <= 0
            ? {}
            : { postedAt: message.postedAt }),
        });
        break;
      case 'postResult':
        if (this.pending?.id !== message.id) {
          break;
        }
        if (message.error === undefined || message.error === '') {
          this.pending.resolve();
        } else {
          this.pending.reject(new Error(message.error));
        }
        this.pending = null;
        break;
      default:
        break;
    }
  }

  private reconnect(): void {
    this.teardown();
    if (this.closed) {
      return;
    }
    const retryIn = retryDelays[Math.min(this.attempt, retryDelays.length - 1)] ?? 30;
    this.attempt += 1;
    this.handlers.onStatus({ kind: 'disconnected', reason: 'CONNECTION_CLOSED', retryIn });
    this.cancelRetry = this.setTimer(() => this.connect(), retryIn * 1000);
  }

  private teardown(): void {
    this.cancelRetry?.();
    this.cancelRetry = null;
    this.pending?.reject(new Error('disconnected'));
    this.pending = null;
    const socket = this.socket;
    this.socket = null;
    this.opened = false;
    socket?.close();
  }
}
