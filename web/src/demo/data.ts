import {
  channelTypes,
  generatePrograms,
  matchProgram,
  services,
  type DemoProgram,
  type GenreInput,
  type ProgramConditionsInput,
} from './epg';
import type { EventHub } from './events';

// demoUser は、デモでログインしているものとして扱うユーザー。
const demoUser = {
  __typename: 'User',
  id: 'User:demo',
  username: 'demo',
  displayName: 'デモ',
  role: 'ADMIN',
  disabled: false,
  createdAt: '2026-10-01T00:00:00Z',
  avatarUrl: null,
};

const emptyPageInfo = {
  hasNextPage: false,
  hasPreviousPage: false,
  startCursor: null,
  endCursor: null,
};

// paginate は、items を、after (前のページの最後の cursor) の次から first 件だけの connection にする。
// cursor は、items の中の位置を文字列にしたもの。
function paginate<T>(items: T[], first: number, after: string | null | undefined) {
  const start = after == null ? 0 : Number.parseInt(after, 10) + 1;
  const edges = items
    .slice(start, start + first)
    .map((node, index) => ({ cursor: String(start + index), node }));

  return {
    edges,
    pageInfo: {
      hasNextPage: start + first < items.length,
      hasPreviousPage: start > 0,
      startCursor: edges[0]?.cursor ?? null,
      endCursor: edges.at(-1)?.cursor ?? null,
    },
  };
}

type GuideViewInput = {
  name: string;
  genres?: GenreInput[] | null;
  excludeGenres?: GenreInput[] | null;
  minDurationSeconds?: number | null;
  services?: string[] | null;
  hideEmptyServices: boolean;
  freeOnly: boolean;
};

const genreCondition = (genre: GenreInput) => ({ lv1: genre.lv1, lv2: genre.lv2 ?? null });

const servicesOf = (ids: readonly string[]) =>
  ids.flatMap((id) => services.filter((item) => item.id === id));

// createRootValue は、デモの GraphQL の rootValue を作る。Query、Mutation、Subscription のフィールドを 1 つにまとめて持つ。
// 状態はメモリ上だけに持ち、画面を読み直すと元に戻る。
export function createRootValue(events: EventHub, now: number = Date.now()): object {
  const programs = generatePrograms(now);
  const programsById = new Map(programs.map((item) => [item.id, item]));
  const watchReserved = new Set<string>();
  let pinned = [services[0]!.id, services[1]!.id];
  const guideViews = new Map<string, GuideViewInput>();
  let nextGuideView = 1;
  let viewerSettingsVersion = 0;

  // program は、番組に、デモの状態から決まるフィールドを足して返す。
  const program = (item: DemoProgram, archived = false) => ({
    ...item,
    archived,
    watchReserved: watchReserved.has(item.id),
    reservation: null,
  });

  const guideView = (id: string, input: GuideViewInput) => ({
    __typename: 'GuideView',
    id,
    name: input.name,
    genres: (input.genres ?? []).map(genreCondition),
    excludeGenres: (input.excludeGenres ?? []).map(genreCondition),
    minDurationSeconds: input.minDurationSeconds ?? null,
    services: servicesOf(input.services ?? []),
    hideEmptyServices: input.hideEmptyServices,
    freeOnly: input.freeOnly,
  });

  // node は、node(id) で引く。ID の頭に型の名前を付けて、型をまたいで重ならないようにしている。
  const node = (id: string) => {
    const found = programsById.get(id);
    if (found !== undefined) {
      return program(found, found.end <= now);
    }
    const view = guideViews.get(id);
    if (view !== undefined) {
      return guideView(id, view);
    }

    return services.find((item) => item.id === id) ?? null;
  };

  // viewerSettingsChanged は、ピン留めや番組表ビューを変えたことを、画面に知らせる。
  const viewerSettingsChanged = () => {
    viewerSettingsVersion += 1;
    events.publish({ dataChange: null, viewerSettingsVersion, unreadNotificationCount: null });
  };

  return {
    // Query
    viewer: () => demoUser,
    version: () => ({ version: 'demo', revision: null, modified: false, latestRelease: null }),
    timeZone: () => 'Asia/Tokyo',
    tunerServerHealth: () => ({
      degraded: false,
      degradedSince: null,
      clockSkewed: false,
      clockSkewMs: null,
    }),
    channelTypes: () => channelTypes,
    node: ({ id }: { id: string }) => node(id),
    services: () => services,
    pinnedServices: () => servicesOf(pinned),
    guideViews: () => [...guideViews].map(([id, input]) => guideView(id, input)),
    guide: (args: {
      from: string;
      to: string;
      services?: string[] | null;
      view?: string | null;
    }) => {
      const from = Date.parse(args.from);
      const to = Date.parse(args.to);
      const view = args.view == null ? undefined : guideViews.get(args.view);
      const ids = args.services ?? view?.services ?? [];
      const conditions: ProgramConditionsInput = {
        genres: view?.genres,
        excludeGenres: view?.excludeGenres,
        minDurationSeconds: view?.minDurationSeconds,
        freeOnly: view?.freeOnly,
      };

      return (ids.length > 0 ? servicesOf(ids) : services)
        .map((service) => ({
          service,
          programs: programs
            .filter(
              (item) =>
                item.service === service &&
                item.start < to &&
                item.end > from &&
                matchProgram(item, conditions),
            )
            .map((item) => program(item)),
        }))
        .filter((row) => view?.hideEmptyServices !== true || row.programs.length > 0);
    },
    searchPrograms: (args: {
      conditions: ProgramConditionsInput;
      archive?: { from?: string | null; to?: string | null } | null;
      first: number;
      after?: string | null;
    }) => {
      const archive = args.archive;
      const archiveTo = archive?.to == null ? now : Date.parse(archive.to);
      const archiveFrom =
        archive?.from == null ? archiveTo - 30 * 24 * 60 * 60 * 1000 : Date.parse(archive.from);
      // archive を指定すると、放送が終わった番組を、指定しなければ、これから放送する番組を探す。
      const found = programs
        .filter((item) =>
          archive == null
            ? item.end > now
            : item.end <= now && item.end > archiveFrom && item.start < archiveTo,
        )
        .filter((item) => matchProgram(item, args.conditions))
        .map((item) => program(item, archive != null));

      return paginate(found, args.first, args.after);
    },
    continueWatching: () => [],
    recordings: () => ({ edges: [], pageInfo: emptyPageInfo }),
    reservations: () => ({ edges: [], pageInfo: emptyPageInfo }),

    // Mutation
    setServicePinned: ({ id, pinned: value }: { id: string; pinned: boolean }) => {
      pinned = pinned.filter((item) => item !== id);
      if (value) {
        pinned.push(id);
      }
      viewerSettingsChanged();

      return servicesOf(pinned);
    },
    createGuideView: ({ input }: { input: GuideViewInput }) => {
      const id = `GuideView:${nextGuideView}`;
      nextGuideView += 1;
      guideViews.set(id, input);
      viewerSettingsChanged();

      return guideView(id, input);
    },
    updateGuideView: ({ id, input }: { id: string; input: GuideViewInput }) => {
      if (!guideViews.has(id)) {
        throw new Error('the guide view is not found');
      }
      guideViews.set(id, input);
      viewerSettingsChanged();

      return guideView(id, input);
    },
    deleteGuideView: ({ id }: { id: string }) => {
      guideViews.delete(id);
      viewerSettingsChanged();

      return id;
    },
    setWatchReservation: ({ program: id, reserved }: { program: string; reserved: boolean }) => {
      if (reserved) {
        watchReserved.add(id);
      } else {
        watchReserved.delete(id);
      }

      return node(id);
    },

    // Subscription。graphql-js は、流れた値を rootValue にして選択集合を解くので、フィールド名で包む。
    viewerEvents: () => events.listen((event) => ({ viewerEvents: event })),
  };
}
