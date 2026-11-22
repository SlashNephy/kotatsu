// 画面の出し分けの Story (#2056) で、デモの既定の応答を一部だけ変える部品。PageStory の overrides に関数として渡す。
import type { DemoRoot } from '../demo';

type Edge = { node: Record<string, unknown>; cursor: string };
type Connection = { edges: Edge[]; pageInfo: Record<string, unknown> };
type ReservationArgs = { states?: string[] | null } & Record<string, unknown>;

// call は、デモの既定の応答のフィールド name を、args で呼ぶ。
export function call<T>(root: DemoRoot, name: string, args: object = {}): T {
  const field = root[name];
  if (typeof field !== 'function') {
    throw new Error(`the demo has no field ${name}`);
  }

  return (field as (args: object) => T)(args);
}

// reservationsWith は、予約の一覧のうち、始まりの早い count 件の割り当ての状態を state (CONFLICT、PARTIAL) にする。
// 状態で絞る問い合わせ (states) には、変えた予約だけを返す。more なら、状態で絞った一覧に続きがあることにする (100 件を超える競合)。
export function reservationsWith(state: string, count: number, more = false) {
  return (root: DemoRoot) => ({
    reservations: (args: ReservationArgs) => {
      const { states, ...rest } = args;
      const all = call<Connection>(root, 'reservations', { ...rest, first: 1000 });
      const edges = all.edges.map((edge, index) =>
        index < count
          ? {
              ...edge,
              node: {
                ...edge.node,
                assignment: { ...(edge.node['assignment'] as object), state },
              },
            }
          : edge,
      );
      const listed =
        states == null
          ? edges
          : edges.filter((edge) =>
              states.includes((edge.node['assignment'] as { state: string }).state),
            );
      const page = listed.slice(0, Number(rest['first'] ?? listed.length));

      // 続きの有無と終わりの位置は、返す 1 ページから決め直す (1000 件で引いた一覧のままにしない)。
      return {
        edges: page,
        pageInfo: {
          ...all.pageInfo,
          hasNextPage: more || page.length < listed.length,
          endCursor: page.at(-1)?.cursor ?? null,
        },
      };
    },
  });
}

// asViewer は、ログイン中のユーザーを、ロール role (VIEWER、MEMBER) にする。管理者でないときの出し分けを見るのに使う。
export function asViewer(role: string) {
  return (root: DemoRoot) => ({
    viewer: () => ({ ...call<object>(root, 'viewer'), role }),
  });
}

// merge は、関数の overrides を重ねる。後のものが優先する。
export function merge(...overrides: ((root: DemoRoot) => object)[]) {
  return (root: DemoRoot) => Object.assign({}, ...overrides.map((override) => override(root)));
}

// RecordingPatch は、録画の差し替えるフィールド。関数なら、デモの録画を受け取って差し替えるフィールドを返す。
type RecordingPatch = Record<string, unknown> | ((recording: Record<string, unknown>) => object);

function apply(recording: Record<string, unknown>, patch: RecordingPatch) {
  return { ...recording, ...(typeof patch === 'function' ? patch(recording) : patch) };
}

// recordingsWith は、録画の一覧の新しい順に、patches を 1 つずつ当てる。録画の詳細 (node) と、続きから見る録画にも同じものを当てる。
// 録画の詳細の Story は、いちばん新しい録画 (firstRecordingPath) を開くので、patches[0] がその録画に当たる。
export function recordingsWith(...patches: RecordingPatch[]) {
  return (root: DemoRoot) => {
    const newest = call<Connection>(root, 'recordings', { first: 1000, order: 'NEWEST_FIRST' });
    const byId = new Map<string, RecordingPatch>();
    newest.edges.forEach((edge, index) => {
      const patch = patches[index];
      if (patch !== undefined) {
        byId.set(String(edge.node['id']), patch);
      }
    });
    const patched = (recording: Record<string, unknown> | null) => {
      const patch = recording === null ? undefined : byId.get(String(recording['id']));

      return recording === null || patch === undefined ? recording : apply(recording, patch);
    };

    return {
      recordings: (args: object) => {
        const result = call<Connection>(root, 'recordings', args);

        return {
          ...result,
          edges: result.edges.map((edge) => ({ ...edge, node: patched(edge.node) })),
        };
      },
      continueWatching: (args: object) =>
        call<Record<string, unknown>[]>(root, 'continueWatching', args).map(patched),
      node: (args: { id: string }) =>
        patched(call<Record<string, unknown> | null>(root, 'node', args)),
    };
  };
}

// minutesAgo は、今から minutes 分前の時刻 (ISO 8601)。
export function minutesAgo(minutes: number): string {
  return new Date(Date.now() - minutes * 60_000).toISOString();
}

// replaceProperty は、target の name を value に差し替え、元に戻す関数を返す。Story の beforeEach で、ブラウザの機能
// (Web Push や WebAuthn など) がない端末や、Service Worker の状態を作るのに使う。
export function replaceProperty(target: object, name: string, value: unknown): () => void {
  const original = Object.getOwnPropertyDescriptor(target, name);
  Object.defineProperty(target, name, { value, configurable: true, writable: true });

  return () => {
    if (original === undefined) {
      Reflect.deleteProperty(target, name);
    } else {
      Object.defineProperty(target, name, original);
    }
  };
}

// removeProperty は、target から name を消し、元に戻す関数を返す。'name' in target で機能の有無を見る所に、ない端末を作る。
export function removeProperty(target: object, name: string): () => void {
  const original = Object.getOwnPropertyDescriptor(target, name);
  Reflect.deleteProperty(target, name);

  return () => {
    if (original !== undefined) {
      Object.defineProperty(target, name, original);
    }
  };
}

// withStorage は、localStorage に entries を書き、元に戻す関数を返す。端末ごとの設定 (プレイヤーの拡大や補間など) を変えた
// 画面を見るのに使う。
export function withStorage(entries: Record<string, string>): () => void {
  const previous = Object.keys(entries).map((key) => [key, localStorage.getItem(key)] as const);
  for (const [key, value] of Object.entries(entries)) {
    localStorage.setItem(key, value);
  }

  return () => {
    for (const [key, value] of previous) {
      if (value === null) {
        localStorage.removeItem(key);
      } else {
        localStorage.setItem(key, value);
      }
    }
  };
}

// all は、beforeEach の後片付けをまとめる。
export function all(...cleanups: (() => void)[]): () => void {
  return () => {
    // 配列を並べ替えずに、後ろから片付ける。2 度呼ばれても、同じ順に片付ける。
    for (const cleanup of cleanups.toReversed()) {
      cleanup();
    }
  };
}

// nodeWith は、ID が id のノード (サービスや番組) のフィールドを、patch で差し替える。
export function nodeWith(id: string, patch: Record<string, unknown>) {
  return (root: DemoRoot) => ({
    node: (args: { id: string }) => {
      const node = call<Record<string, unknown> | null>(root, 'node', args);

      return node !== null && args.id === id ? { ...node, ...patch } : node;
    },
  });
}

// liveStreamScrambled は、afterMs 後に、サービス service がスクランブルされたまま届くと決めたという知らせを流す。
// 知らせのほかは、デモの既定の知らせのまま流す。
export function liveStreamScrambled(service: string, afterMs = 3_000) {
  return (root: DemoRoot) => ({
    viewerEvents: async function* () {
      await new Promise((resolve) => setTimeout(resolve, afterMs));
      yield {
        viewerEvents: { liveStreamScrambled: { service, decidedAt: new Date().toISOString() } },
      };
      yield* call<AsyncIterable<unknown>>(root, 'viewerEvents');
    },
  });
}
