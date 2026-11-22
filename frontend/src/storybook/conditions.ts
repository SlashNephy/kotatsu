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
