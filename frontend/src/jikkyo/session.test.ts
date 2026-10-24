import { describe, expect, test } from 'vitest';

import { NXJikkyoSession, type LiveComment, type SessionStatus } from './session';

// FakeSocket は、送ったものを覚え、テストからイベントを起こせる WebSocket の代わり。
class FakeSocket {
  readonly url: string;
  readonly sent: unknown[] = [];
  closed = false;
  private readonly listeners = new Map<string, ((event: { data?: string }) => void)[]>();

  constructor(url: string) {
    this.url = url;
  }

  send(data: string): void {
    this.sent.push(JSON.parse(data));
  }

  close(): void {
    this.closed = true;
  }

  addEventListener(type: string, listener: (event: { data?: string }) => void): void {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), listener]);
  }

  emit(type: string, data?: unknown): void {
    for (const listener of this.listeners.get(type) ?? []) {
      listener(data === undefined ? {} : { data: JSON.stringify(data) });
    }
  }
}

// setup は、偽の WebSocket とタイマーでセッションを作る。
function setup() {
  const sockets: FakeSocket[] = [];
  const timers: { ms: number; callback: () => void; cancelled: boolean }[] = [];
  const comments: LiveComment[] = [];
  const statuses: SessionStatus[] = [];
  const session = new NXJikkyoSession(
    'jk1',
    { onComment: (comment) => comments.push(comment), onStatus: (status) => statuses.push(status) },
    {
      createSocket: (url) => {
        const socket = new FakeSocket(url);
        sockets.push(socket);

        return socket as unknown as WebSocket;
      },
      now: () => 1000,
      setTimer: (callback, ms) => {
        const timer = { ms, callback, cancelled: false };
        timers.push(timer);

        return () => {
          timer.cancelled = true;
        };
      },
    },
  );

  return { session, sockets, timers, comments, statuses };
}

// connect は、視聴セッションとコメントセッションをつなぎ、直近のコメントを受け取り終えたところまで進める。
function connect(env: ReturnType<typeof setup>) {
  const [watch] = env.sockets;
  watch?.emit('open');
  watch?.emit('message', {
    type: 'room',
    data: {
      messageServer: { uri: 'wss://nx-jikkyo.tsukumijima.net/api/v1/channels/jk1/ws/comment' },
      threadId: '27359',
      yourPostKey: 'key',
      vposBaseTime: '2026-10-11T04:00:00+09:00',
    },
  });
  const comment = env.sockets[1];
  comment?.emit('open');

  return { watch, comment };
}

describe('NXJikkyoSession', () => {
  test('視聴セッションを開き、room で知らされたコメントセッションに直近の 100 件を求める', () => {
    const env = setup();
    const { watch, comment } = connect(env);

    expect(watch?.url).toBe('wss://nx-jikkyo.tsukumijima.net/api/v1/channels/jk1/ws/watch');
    expect(watch?.sent).toEqual([{ type: 'startWatching', data: { reconnect: false } }]);
    expect(comment?.url).toBe('wss://nx-jikkyo.tsukumijima.net/api/v1/channels/jk1/ws/comment');
    expect(comment?.sent[0]).toContainEqual({
      thread: {
        version: '20061206',
        thread: '27359',
        threadkey: 'key',
        user_id: '',
        res_from: -100,
      },
    });
  });

  test('直近のコメントは流さず、rf:0 の後に届いたコメントだけを流す。自分の投稿は流さない', () => {
    const env = setup();
    const { comment } = connect(env);
    comment?.emit('message', { chat: { content: '過去', mail: '184' } });
    comment?.emit('message', { ping: { content: 'rf:0' } });
    comment?.emit('message', { chat: { content: '今', mail: '184 shita' } });
    comment?.emit('message', { chat: { content: '自分', mail: '184', yourpost: 1 } });

    expect(env.comments).toEqual([{ content: '今', mail: '184 shita', at: 1000 }]);
    expect(env.statuses.at(-1)).toEqual({ kind: 'connected' });
  });

  test('ping に pong を返し、言われた間隔で keepSeat を送る', () => {
    const env = setup();
    const { watch } = connect(env);
    watch?.emit('message', { type: 'ping' });
    watch?.emit('message', { type: 'seat', data: { keepIntervalSec: 30 } });
    const keep = env.timers.find((timer) => timer.ms === 30_000);
    keep?.callback();

    expect(watch?.sent.slice(1)).toEqual([{ type: 'pong' }, { type: 'keepSeat' }]);
  });

  test('切れたら、間隔を延ばしながらつなぎ直す', () => {
    const env = setup();
    const { watch } = connect(env);
    watch?.emit('message', { type: 'disconnect', data: { reason: 'TAKEOVER' } });

    expect(watch?.closed).toBe(true);
    expect(env.statuses.at(-1)).toEqual({ kind: 'disconnected', reason: 'TAKEOVER', retryIn: 3 });
    env.timers.at(-1)?.callback();
    expect(env.sockets).toHaveLength(3);
    env.sockets[2]?.emit('close');
    expect(env.statuses.at(-1)).toEqual({
      kind: 'disconnected',
      reason: 'CONNECTION_CLOSED',
      retryIn: 5,
    });
  });

  test('投稿は、postCommentResult で成功し、COMMENT_ で始まるエラーで失敗する', async () => {
    const env = setup();
    const { watch, comment } = connect(env);
    comment?.emit('message', { ping: { content: 'rf:0' } });

    const posted = env.session.post('こんにちは');
    expect(watch?.sent.at(-1)).toMatchObject({
      type: 'postComment',
      data: { text: 'こんにちは', isAnonymous: true },
    });
    watch?.emit('message', { type: 'postCommentResult' });
    await expect(posted).resolves.toBeUndefined();

    const rejected = env.session.post('もう一度');
    watch?.emit('message', { type: 'error', data: { code: 'COMMENT_POST_NOT_ALLOWED' } });
    await expect(rejected).rejects.toThrow('COMMENT_POST_NOT_ALLOWED');
    // 投稿の失敗では、つなぎ直さない。
    expect(watch?.closed).toBe(false);
  });

  test('閉じた後は、遅れて届いたイベントでつなぎ直さない', () => {
    const env = setup();
    const { watch } = connect(env);
    env.session.close();
    watch?.emit('close');

    expect(env.sockets).toHaveLength(2);
    expect(env.statuses.at(-1)).not.toMatchObject({ kind: 'disconnected' });
  });
});

describe('NXJikkyoSession の座席とスレッド', () => {
  test('seat が届き直しても、keepSeat の繰り返しは 1 つだけにする', () => {
    const env = setup();
    const { watch } = connect(env);
    watch?.emit('message', { type: 'seat', data: { keepIntervalSec: 30 } });
    watch?.emit('message', { type: 'seat', data: { keepIntervalSec: 30 } });

    const active = env.timers.filter((timer) => timer.ms === 30_000 && !timer.cancelled);
    expect(active).toHaveLength(1);
  });

  test('room が届き直したら、前のコメントセッションを閉じ、前のセッションのイベントではつなぎ直さない', () => {
    const env = setup();
    const { watch, comment } = connect(env);
    watch?.emit('message', {
      type: 'room',
      data: {
        messageServer: { uri: 'wss://nx-jikkyo.tsukumijima.net/api/v1/channels/jk1/ws/comment2' },
        threadId: '27360',
        vposBaseTime: '2026-10-12T04:00:00+09:00',
      },
    });
    comment?.emit('close');

    expect(comment?.closed).toBe(true);
    expect(env.sockets[2]?.url).toBe(
      'wss://nx-jikkyo.tsukumijima.net/api/v1/channels/jk1/ws/comment2',
    );
    expect(env.statuses.at(-1)).not.toMatchObject({ kind: 'disconnected' });
  });

  test.each([
    ['別のホスト', 'wss://evil.example/comment'],
    ['ホスト名の続き', 'wss://nx-jikkyo.tsukumijima.net.evil.example/comment'],
    ['別のポート', 'wss://nx-jikkyo.tsukumijima.net:8443/comment'],
    ['暗号化しない接続', 'ws://nx-jikkyo.tsukumijima.net/comment'],
  ])('room の接続先が NX-Jikkyo でなければ (%s)、接続せずにつなぎ直す', (_, uri) => {
    const env = setup();
    const [watch] = env.sockets;
    watch?.emit('open');
    watch?.emit('message', {
      type: 'room',
      data: {
        messageServer: { uri },
        threadId: '27359',
        vposBaseTime: '2026-10-11T04:00:00+09:00',
      },
    });

    expect(env.sockets.map((socket) => socket.url)).not.toContain(uri);
    expect(watch?.closed).toBe(true);
    expect(env.statuses.at(-1)).toEqual({
      kind: 'disconnected',
      reason: 'INVALID_MESSAGE_SERVER',
      retryIn: 3,
    });
  });
});
