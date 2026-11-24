import { GraphQLError } from 'graphql';
import {
  demoAnimeEpisodes,
  demoAnimeRecordApps,
  demoAnimeLibrary,
  demoAnimeWishlist,
  demoAnimeRecordSettings,
} from './anime';
import { createDemoCaptures, type DemoCaptures } from './captures';
import {
  channelTypes,
  generatePrograms,
  matchProgram,
  services,
  type DemoProgram,
  type GenreInput,
  type ProgramConditionsInput,
} from './epg';
import { createEventHub, type EventHub } from './events';
import {
  defaultFileNameTemplate,
  fileNameTemplateItems,
  fileNameTemplatePresets,
  renderFileName,
  sampleFileNameData,
} from './filename';
import { createRecordings, type RecordingFilter } from './recordings';
import { createScheduler, type RuleInput, type SettingsInput } from './schedule';

// demoDataChannels は、購読を始めたときに今のバージョンを送る、データの種類 (GraphQL の DataChannel)。
// デモのユーザーは管理者なので、管理者だけに送る種類 (ADMIN) も含める。
const demoDataChannels = [
  'EPG',
  'RULES',
  'RESERVATIONS',
  'ASSIGNMENTS',
  'RESERVATION_CONFLICTS',
  'TUNERS',
  'RECORDINGS',
  'ANIME_FOLLOWING',
  'DRAMA_WATCHLIST',
  'ADMIN',
] as const;

// notFound は、呼ばれると、見つからない (NOT_FOUND) のエラーを返す応答を返す。
const notFound = (message: string) => () => {
  throw new GraphQLError(message, { extensions: { code: 'NOT_FOUND' } });
};

// notificationKinds は、通知の種類 (GraphQL の NotificationKind)。
const notificationKinds = [
  'RECORDING_STARTED',
  'RECORDING_COMPLETED',
  'RECORDING_FAILED',
  'RESERVATION_CONFLICT',
  'STORAGE_LOW',
  'TUNER_SERVER_UNREACHABLE',
  'WATCH_REMINDER',
  'ANIME_RECORDED',
  'ANIME_RECORD_FAILED',
  'DRAMA_RECORDED',
  'DRAMA_RECORD_FAILED',
  'SCRAMBLED_RECORDINGS',
  'LINEUP_CHANGED',
];

// defaultNotificationPreference は、変えていない種類の通知の設定。サーバーと同じく、どれも有効にする。
const defaultNotificationPreference = (kind: string) => ({
  kind,
  inApp: true,
  webPush: true,
  ruleReservations: true,
});

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

// paginate は、items を、after (前のページの最後の cursor) の次から first 件だけの connection にする。
// cursor は、items の中の位置を文字列にしたもの。
export function paginate<T>(items: T[], first: number, after: string | null | undefined) {
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

type WatchRuleInput = {
  name: string;
  enabled?: boolean | null;
  conditions: ProgramConditionsInput;
};

type WatchRule = { input: WatchRuleInput; createdAt: number; updatedAt: number };

const genreCondition = (genre: GenreInput) => ({ lv1: genre.lv1, lv2: genre.lv2 ?? null });

const servicesOf = (ids: readonly string[]) =>
  ids.flatMap((id) => services.filter((item) => item.id === id));

// demoDramaRecordSettings は、デモの、見たドラマの記録の設定 (#1649)。デモは TMDB との連携を使わないので、無効にする。
function demoDramaRecordSettings() {
  return {
    enabled: false,
    thresholdPercent: 20,
    destinations: [{ destination: 'SIMKL', available: false, connection: null }],
  };
}

// createRootValue は、デモの GraphQL の rootValue を作る。Query、Mutation、Subscription のフィールドを 1 つにまとめて持つ。
// 状態はメモリ上だけに持ち、画面を読み直すと元に戻る。
export function createRootValue(
  events: EventHub,
  now: number = Date.now(),
  captures: DemoCaptures = createDemoCaptures(),
): object {
  const programs = generatePrograms(now);
  const programsById = new Map(programs.map((item) => [item.id, item]));
  const watchReserved = new Set<string>();
  let pinned = [services[0]!.id, services[1]!.id];
  const guideViews = new Map<string, GuideViewInput>();
  let nextGuideView = 1;
  // watchRules は、デモのユーザーの視聴ルール (設計書 4.8)。watchRuleReserved は、視聴ルールが付けた視聴予約の番組。
  const watchRules = new Map<string, WatchRule>();
  const watchRuleReserved = new Set<string>();
  let nextWatchRule = 1;
  let viewerSettingsVersion = 0;
  let minutesBefore = 5;
  const dataVersions = new Map<string, number>();
  // dataChanged は、予約やルールを変えたことを、変わった種類ごとにバージョンを上げて画面に通知する。
  const dataChanged = (channels: string[]) => {
    for (const channel of channels) {
      const version = (dataVersions.get(channel) ?? 0) + 1;
      dataVersions.set(channel, version);
      events.publish({
        dataChange: { channel, version },
        viewerSettingsVersion: null,
        unreadNotificationCount: null,
        streamsDraining: null,
        liveStreamScrambled: null,
      });
    }
  };
  // notificationPreferences は、通知の種類ごとに保存した設定 (#2073)。
  const notificationPreferences = new Map<
    string,
    { kind: string; inApp: boolean; webPush: boolean; ruleReservations: boolean }
  >();
  // systemSettings は、全体の設定 (#2073)。管理の画面で保存すると書き換わる。
  let systemSettings: Record<string, unknown> = {
    recordingDefaults: {
      poolId: 'default',
      fileNameTemplate: defaultFileNameTemplate,
      startMarginSeconds: 0,
      endMarginSeconds: 0,
      priority: 0,
      joinSegments: true,
      detectCm: false,
      // 字幕の自動生成 (#1682) は、既定では使わない。
      generateCaptions: false,
      transcodePreset: null,
      // 変換で出すコマ数の既定値 (#1509)。
      transcodeFrameRate: 'auto',
    },
    archiveRetentionDays: 30,
    notificationRetentionDays: 30,
    ruleDefaults: {
      avoidDuplicates: false,
      duplicateWindowDays: null,
      followEventRelay: true,
      bundleSameEpisodes: true,
    },
    // デモには tenban がないので、データ放送の文書の通信は中継しない (#501)。
    dataBroadcastIpEnabled: false,
    // デモでは、外部プレイヤーと IPTV のクライアントの URL を発行する操作も見せる (#1755)。
    externalPlayerUrlsEnabled: true,
    iptvFeedsEnabled: true,
    // デモでは、録画のダウンロードの操作も見せる (#1762)。
    recordingDownloadsEnabled: true,
    cmDetector: 'FFMPEG',
    tvtplayChaptersEnabled: false,
    // デモでは、API トークンを発行する操作も見せる (#1769)。
    epgstationApiEnabled: true,
    // デモは外部へ問い合わせないので、しょぼいカレンダー、Annict、AniList との連携は使わない (設計書 3.8)。
    animeLinkingEnabled: false,
    // TMDB との連携も、外部へ問い合わせるので使わない (#1651)。
    dramaLinkingEnabled: false,
    // デモは外部につながず、架空のコメントを流す。実況の画面を見せるため、全体では有効にし、ユーザーの設定は切っておく (設計書 7.4)。
    jikkyoEnabled: true,
    commentSources: ['NX_JIKKYO'],
    // デモにはタイムシフトのディレクトリがない。
    timeshiftWhileWatching: false,
    timeshiftWhileWatchingRetentionSeconds: 3600,
    historyMaxDrops: 0,
    updatedAt: null,
    // 以下は、サーバーの既定値 (backend/internal/usecase/settings の DefaultXxx と defaultSettings) にそろえる (#2073)。
    watchProgressRetentionDays: 365,
    // デモは外部と通信しないので、通信の記録は取らない。
    networkActivityEnabled: false,
    networkActivityRetentionDays: 30,
    mcpEnabled: false,
    drcsReplacements: [],
    auditRetentionDays: 365,
    auditClientInfoRetentionDays: 90,
    // デモは外部へ問い合わせないので、漏えいしたパスワードの照合は使わない。
    hibpEnabled: false,
    // 本物のサーバーは、設定していなくても空の文字列を返す。
    webPushSubject: '',
    thumbnailPreviewsEnabled: true,
    sleepControlEnabled: true,
    captionThreads: 0,
    jikkyoChannels: [],
  };
  const scheduler = createScheduler({ programs, now, user: demoUser, changed: dataChanged });
  const recordings = createRecordings({
    programs,
    now,
    rules: scheduler.recordingRules(),
    ruleView: scheduler.findRule,
    changed: dataChanged,
  });
  let playerSettings = {
    jikkyoEnabled: false,
    commentSources: [] as string[],
    captionsEnabled: true,
    comments: {
      opacity: 1,
      speed: 1,
      scale: 1,
      showAll: false,
      mutedKeywords: [],
      mutedUsers: [],
      mutedCommands: [],
      mixOtherBroadcasts: false,
    },
    captions: {
      superimposeLive: true,
      superimposeRecorded: true,
      stroke: true,
      backgroundOpacity: null,
    },
    // preferences は、端末をまたいで同期する設定 (#1862)。キーの順に並べる。
    preferences: [] as { key: string; value: string }[],
  };
  let nextPlayer = 1;
  // animeOverrides は、サービスごとの、しょぼいカレンダーと Annict のチャンネルの対応の上書き (設計書 3.8)。
  const animeOverrides = new Map<string, { syobocal: number | null; annict: number | null }>();
  const animeChannel = (serviceId: string) => {
    const override = animeOverrides.get(serviceId) ?? { syobocal: null, annict: null };

    return {
      serviceId,
      definitionName: null,
      importedSyobocalChannelId: null,
      importedAnnictChannelId: null,
      syobocalChannelIdOverride: override.syobocal,
      annictChannelIdOverride: override.annict,
      syobocalChannelId: override.syobocal,
      annictChannelId: override.annict,
    };
  };
  // idle は、何も流さない購読に使う。ほかの端末からのプレイヤーの操作は、デモでは届かない。
  const idle = createEventHub();

  // program は、番組に、デモの状態から決まるフィールドを足して返す。
  // 放送が終わった番組は、アーカイブに移したものとして扱う。
  const program = (item: DemoProgram) => ({
    ...item,
    archived: item.end <= now,
    watchReserved: watchReserved.has(item.id),
    // 予約は、選ばれたときだけ引く。番組表の番組ごとに割り当てを計算しないため。
    reservation: () => scheduler.reservationOfProgram(item.id),
    // デモの録画は番組表の番組と結び付けないので、番組の録画はない (#1121)。
    recordings: [],
    recorded: false,
    animeEpisodes: demoAnimeEpisodes(item),
    // デモは TMDB に問い合わせないので、ドラマの作品には結びつけない (#1651)。
    dramaEpisode: null,
    dramaTitleKey: item.name,
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

  const watchRuleView = (id: string, rule: WatchRule) => ({
    __typename: 'WatchRule',
    id,
    name: rule.input.name,
    enabled: rule.input.enabled !== false,
    conditions: scheduler.conditionsView(rule.input.conditions),
    createdAt: new Date(rule.createdAt).toISOString(),
    updatedAt: new Date(rule.updatedAt).toISOString(),
  });

  // applyWatchRules は、ルールの照合 (設計書 4.8) を簡単にしたもの。有効な視聴ルールに一致したこれからの番組に視聴予約を付け、
  // 視聴ルールが付けた視聴予約のうち、一致しなくなったものを外す。手で外した番組を保持することは、デモでは省く。
  const applyWatchRules = () => {
    const enabled = [...watchRules.values()].filter((rule) => rule.input.enabled !== false);
    const matched = new Set(
      programs
        .filter(
          (item) =>
            item.start > now && enabled.some((rule) => matchProgram(item, rule.input.conditions)),
        )
        .map((item) => item.id),
    );
    for (const id of watchRuleReserved) {
      if (!matched.has(id)) {
        watchReserved.delete(id);
        watchRuleReserved.delete(id);
      }
    }
    for (const id of matched) {
      if (!watchReserved.has(id)) {
        watchReserved.add(id);
        watchRuleReserved.add(id);
      }
    }
  };

  // node は、node(id) で取得する。ID のプレフィックスに型の名前を付けて、型をまたいで重ならないようにしている。
  const node = (id: string) => {
    const found = programsById.get(id);
    if (found !== undefined) {
      return program(found);
    }
    const view = guideViews.get(id);
    if (view !== undefined) {
      return guideView(id, view);
    }
    const watchRule = watchRules.get(id);
    if (watchRule !== undefined) {
      return watchRuleView(id, watchRule);
    }
    const scheduled = scheduler.node(id) ?? recordings.node(id);
    if (scheduled !== undefined) {
      return scheduled;
    }

    return services.find((item) => item.id === id) ?? null;
  };

  // viewerSettingsChanged は、ピン留め、番組表ビュー、視聴予約を変えたことを、画面に通知する。
  const viewerSettingsChanged = () => {
    viewerSettingsVersion += 1;
    events.publish({
      dataChange: null,
      viewerSettingsVersion,
      unreadNotificationCount: null,
      streamsDraining: null,
      liveStreamScrambled: null,
    });
  };

  return {
    // Query
    viewer: () => demoUser,
    // 管理と設定の画面の応答 (#2073)。デモはデモのユーザー 1 人だけで、ログインしている端末もこのブラウザだけにする。
    users: () => [demoUser],
    sessions: () => [
      {
        __typename: 'Session',
        id: 'Session:demo',
        current: true,
        createdAt: demoUser.createdAt,
        lastSeenAt: new Date(now).toISOString(),
        userAgent: globalThis.navigator?.userAgent ?? '',
        sourceIp: null,
      },
    ],
    // デモのユーザーは、第二要素を登録していない。
    secondFactors: () => ({ totp: false, remainingRecoveryCodes: 0 }),
    // デモは Web Push を送らないので、公開鍵は空にする。
    webPushPublicKey: () => '',
    // デモにはタイムシフトのディレクトリがない。
    timeshiftAvailable: () => false,
    drcsReplacements: () => systemSettings['drcsReplacements'],
    // デモは外部へ問い合わせないので、アニメの取り込みのトークンも登録していない。
    animeImportToken: () => ({ configured: false, updatedAt: null }),
    // 通知の種類ごとの設定 (設計書 6.2)。サーバーと同じく、変えていない種類は、どれも有効にする。保存すれば保持する。
    notificationPreferences: () =>
      notificationKinds.map(
        (kind) => notificationPreferences.get(kind) ?? defaultNotificationPreference(kind),
      ),
    updateNotificationPreference: ({
      input,
    }: {
      input: { kind: string; inApp: boolean; webPush: boolean; ruleReservations?: boolean | null };
    }) => {
      const preference = {
        kind: input.kind,
        inApp: input.inApp,
        webPush: input.webPush,
        ruleReservations:
          input.ruleReservations ??
          notificationPreferences.get(input.kind)?.ruleReservations ??
          true,
      };
      notificationPreferences.set(input.kind, preference);

      return preference;
    },
    // デモには、ライブラリ、タイムシフトのバッファ、連携の許可の要求がないので、本物のサーバーと同じく、見つからないと答える。
    libraryDirectory: notFound('library not found'),
    librarySearch: notFound('library not found'),
    libraryFile: notFound('library file not found'),
    timeshiftProgram: notFound('timeshift program not found'),
    oauthAuthorization: notFound('authorization request not found'),
    // デモには、裏で動くジョブ (River) がない (設計書 11.3)。
    jobs: () => ({ nodes: [], endCursor: null }),
    jobCounts: () => [],
    // デモでは、API トークンを発行していない。
    apiTokens: () => [],
    // デモには、パスキー、連携の許可、Web Push の購読、Webhook、タイムシフトで録るサービス、保存先の片付けの対象がない。
    passkeys: () => [],
    oauthGrants: () => [],
    webPushSubscriptions: () => [],
    webhooks: () => [],
    timeshiftChannels: () => [],
    missingRecordings: () => [],
    orphanFiles: () => [],
    recordingRenames: () => [],
    libraryArtifacts: () => ({ size: 0, files: 0 }),
    googleDriveFolderChoices: () => [],
    // 監査ログは記録しない。
    auditEvents: ({ first, after }: { first: number; after?: string | null }) =>
      paginate([], first, after),
    // デモにはログを集めるサーバーがないので、ソースのないログの報告を返す。
    logReport: () => ({ sources: [], redactHosts: [], instanceId: 'demo', roles: [] }),
    // システムの状態 (設計書 11.3)。デモは 1 つのブラウザの中で全部のロールを動かすものとして、架空のチューナーと保存先を見せる。
    systemStatus: () => ({
      instances: [
        {
          instanceId: 'demo',
          role: 'all',
          address: '',
          startedAt: new Date(now).toISOString(),
          heartbeatAt: new Date(now).toISOString(),
          alive: true,
        },
      ],
      leases: [],
      tunerServer: {
        kind: 'MIRAKURUN',
        version: 'demo',
        degraded: false,
        degradedSince: null,
        lastError: '',
        clockSkewMs: 0,
        clockSkewed: false,
        clockSkewThresholdMs: 2000,
        checkedAt: new Date(now).toISOString(),
        broadcastClockSkewMs: 0,
        broadcastClockSkewed: false,
        broadcastClockCheckedAt: new Date(now).toISOString(),
      },
      tuners: [
        { index: 0, name: 'GR0', types: ['GR'], available: true, remote: false, fault: false },
        { index: 1, name: 'GR1', types: ['GR'], available: true, remote: false, fault: false },
        {
          index: 2,
          name: 'BS0',
          types: ['BS', 'CS'],
          available: true,
          remote: false,
          fault: false,
        },
        {
          index: 3,
          name: 'BS1',
          types: ['BS', 'CS'],
          available: true,
          remote: false,
          fault: false,
        },
      ],
      pools: [
        {
          id: 'default',
          path: '/recordings',
          priority: 0,
          minFreeBytes: 50_000_000_000,
          freeBytes: 1_200_000_000_000,
          totalBytes: 4_000_000_000_000,
        },
      ],
      recordingFailures: [],
    }),
    // デモは外部と通信しないので、通信の記録は空にする (設計書 11.8)。
    networkActivity: () => ({
      enabled: systemSettings['networkActivityEnabled'],
      enabledAt: null,
      retentionDays: systemSettings['networkActivityRetentionDays'],
      totalCount: 0,
      totalSentBytes: 0,
      totalReceivedBytes: 0,
      entries: [],
    }),
    captures: (args: { keyword?: string | null }) => captures.list(args),
    capturesAvailable: () => true,
    // デモには、録画として登録していない TS を置くライブラリがない (設計書 5.8)。
    libraries: () => [],
    version: () => ({ version: 'demo', revision: null, modified: false, latestRelease: null }),
    // デモには tenban がないので、本体の Go のモジュールの一覧はない (#1604)。
    goModuleLicenses: () => null,
    timeZone: () => 'Asia/Tokyo',
    tunerServerHealth: () => ({
      degraded: false,
      degradedSince: null,
      clockSkewed: false,
      clockSkewMs: null,
      broadcastClockSkewed: false,
      broadcastClockSkewMs: null,
    }),
    channelTypes: () => channelTypes,
    node: ({ id }: { id: string }) => node(id),
    services: () => services,
    pinnedServices: () => servicesOf(pinned),
    guideViews: () => [...guideViews].map(([id, input]) => guideView(id, input)),
    watchRules: () => [...watchRules].map(([id, rule]) => watchRuleView(id, rule)),
    guide: (args: {
      from: string;
      to: string;
      services?: string[] | null;
      view?: string | null;
    }) => {
      const from = Date.parse(args.from);
      const to = Date.parse(args.to);
      const view = args.view == null ? undefined : guideViews.get(args.view);
      // services を指定すると、その局だけを返す。ビューの局の並びは、並べ替えだけに使い、
      // 並びに無い局は、既定の並びで後ろに置く。
      const viewOrder = servicesOf(view?.services ?? []);
      const rows =
        args.services != null && args.services.length > 0
          ? servicesOf(args.services)
          : [...viewOrder, ...services.filter((item) => !viewOrder.includes(item))];
      const conditions: ProgramConditionsInput = {
        genres: view?.genres,
        excludeGenres: view?.excludeGenres,
        minDurationSeconds: view?.minDurationSeconds,
        freeOnly: view?.freeOnly,
      };

      return rows
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
        // 局をまたいで放送の順に並べる。アーカイブは、新しいものから並べる。
        .toSorted((a, b) => (archive == null ? a.start - b.start : b.start - a.start))
        .map((item) => program(item));

      return paginate(found, args.first, args.after);
    },
    continueWatching: ({ first }: { first: number }) => recordings.continueWatching(first),
    // デモの録画は番組表の番組と結び付けないので、続きの放送 (#1564) は出さない。
    nextEpisodes: () => [],
    // デモには通知がない。通知の一覧と、局の構成の変化の知らせ (LineupBanner) は空にする。
    notifications: (args: { first: number; after?: string | null }) =>
      paginate([], args.first, args.after),
    unreadNotificationCount: () => 0,
    // デモは通知を持たないので、既読にする通知もない。本物のサーバーと同じく、既読にした数を返す。
    markNotificationsRead: () => 0,
    // デモは、ほかの端末のプレイヤーを保持しないので、リモコンで操れるプレイヤーは出さない。
    players: () => [],
    recordings: (args: {
      first: number;
      after?: string | null;
      filter?: RecordingFilter | null;
      order: string;
    }) => paginate(recordings.list(args.filter, args.order), args.first, args.after),
    tags: () => recordings.tags(),
    playerSettings: () => playerSettings,
    // デモには、映像を変換する kyusu がない。
    transcoding: () => ({ available: false, presets: [] }),
    reservations: (
      args: Parameters<typeof scheduler.reservations>[0] & { first: number; after?: string | null },
    ) => paginate(scheduler.reservations(args), args.first, args.after),
    rules: () => scheduler.rules(),
    ruleGroups: () => scheduler.ruleGroups(),
    // デモは過去の番組を持たないので、番組表で過去の日へは遡れない (#1121)。
    oldestArchivedProgramAt: null,
    // デモでは、照合の重さを警告しない。
    ruleWarnings: () => [],
    previewRule: (args: { input: RuleInput; first: number; after?: string | null }) =>
      paginate(scheduler.previewRule(args).map(program), args.first, args.after),
    // 重複の回避で見送った番組は、録画の履歴から求める。デモには録画の履歴がないので、ない。
    skippedDuplicates: () => [],
    watchReservations: () =>
      programs
        .filter((item) => watchReserved.has(item.id) && item.start > now)
        .toSorted((a, b) => a.start - b.start)
        .map((item) => ({
          program: program(item),
          notifyAt: new Date(item.start - minutesBefore * 60 * 1000).toISOString(),
          createdAt: new Date(now).toISOString(),
        })),
    watchReminderSettings: () => ({ minutesBefore }),
    // 録画を終えた後の変換 (#1253) で選べる画質。デモでは変換しないが、設定の画面を出せるようにする。
    transcodePresets: () => [
      '1080p',
      '720p',
      '540p',
      '360p',
      '1080p-hevc',
      '720p-hevc',
      '540p-hevc',
      '360p-hevc',
    ],
    systemSettings: () => systemSettings,
    // 全体の設定を保存する (#2073)。渡した項目だけを書き換え、保存した時刻を残す。読み直しても、保存した値を返す。
    updateSystemSettings: ({ input }: { input: Record<string, unknown> }) => {
      const { ruleDefaults, recordingDefaults, drcsReplacements, jikkyoChannels, ...rest } =
        input as Record<string, unknown> & {
          ruleDefaults?: Record<string, unknown> | null;
          recordingDefaults?: Record<string, unknown> | null;
          drcsReplacements?: { md5: string; text?: string | null }[] | null;
          jikkyoChannels?:
            | { networkId: number; serviceId: number; channelId?: string | null }[]
            | null;
        };
      const defined = (value: Record<string, unknown> | null | undefined) =>
        Object.fromEntries(Object.entries(value ?? {}).filter(([, v]) => v != null));
      // 本物のサーバーと同じく、録画とルールの既定値は、渡した項目だけを書き換える。
      // 重複を避ける期間は、clearDuplicateWindowDays で空 (区切らない) にする。
      const { clearDuplicateWindowDays, ...ruleChanges } = defined(ruleDefaults);
      const changes = defined(rest);
      // 実況を流すかは、本物のサーバーと同じく、コメントの取得元に NX-Jikkyo を含むかから決める。
      if (Array.isArray(changes['commentSources'])) {
        changes['jikkyoEnabled'] = changes['commentSources'].includes('NX_JIKKYO');
      }
      // 外字の置き換えと実況のチャンネルの対応は、渡したものだけを変え、null なら消す (本物のサーバーと同じ)。
      if (drcsReplacements != null) {
        const table = new Map(
          (systemSettings['drcsReplacements'] as { md5: string; text: string }[]).map((row) => [
            row.md5,
            row.text,
          ]),
        );
        for (const { md5, text } of drcsReplacements) {
          if (text == null) {
            table.delete(md5);
          } else {
            table.set(md5, text);
          }
        }
        changes['drcsReplacements'] = [...table]
          .map(([md5, text]) => ({ md5, text }))
          .toSorted((a, b) => a.md5.localeCompare(b.md5));
      }
      if (jikkyoChannels != null) {
        const table = new Map(
          (
            systemSettings['jikkyoChannels'] as {
              networkId: number;
              serviceId: number;
              channelId: string;
            }[]
          ).map((row) => [`${row.networkId}:${row.serviceId}`, row]),
        );
        for (const { networkId, serviceId, channelId } of jikkyoChannels) {
          if (channelId == null) {
            table.delete(`${networkId}:${serviceId}`);
          } else {
            table.set(`${networkId}:${serviceId}`, { networkId, serviceId, channelId });
          }
        }
        changes['jikkyoChannels'] = [...table.values()].toSorted(
          (a, b) => a.networkId - b.networkId || a.serviceId - b.serviceId,
        );
      }
      systemSettings = {
        ...systemSettings,
        ...changes,
        recordingDefaults: {
          ...(systemSettings['recordingDefaults'] as object),
          ...defined(recordingDefaults),
        },
        ruleDefaults: {
          ...(systemSettings['ruleDefaults'] as object),
          ...ruleChanges,
          ...(clearDuplicateWindowDays === true ? { duplicateWindowDays: null } : {}),
        },
        updatedAt: new Date(now).toISOString(),
      };
      dataChanged(['ADMIN']);

      return systemSettings;
    },
    // デモには、saya-definitions から取り込んだ対応がない。管理者の上書きだけを保持する。
    animeChannels: () => services.map((service) => animeChannel(service.id)),
    animeImports: () => [],
    // デモには TMDB のトークンも、作品の指定もない (#1651)。
    dramaLinking: () => ({ tokenConfigured: false, tokenHint: null, tokenUpdatedAt: null }),
    dramaTitleOverrides: () => [],
    // デモは発行した URL を記録しないので、管理者の一覧は空にする (#1757)。
    issuedStreamUrls: () => [],
    animeRecordSettings: () => demoAnimeRecordSettings(),
    animeRecordApps: () => demoAnimeRecordApps(window.location.origin),
    // デモは外部へ問い合わせないので、Google Drive (#1902) は登録も接続もしていないものとして出す。
    googleIntegration: () => ({
      clientId: null,
      configured: false,
      redirectUri: `${window.location.origin}/internal/auth/google/callback`,
      updatedAt: null,
      drive: null,
      folders: [],
    }),
    animeWishlist: () => demoAnimeWishlist(),
    // デモは Annict と AniList に問い合わせないので、フォロー中の人が見ている作品もない (#1968)。
    animeFollowingWatching: () => ({ enabled: false, works: [] }),
    animeLibrary: () => demoAnimeLibrary(),
    // デモは TMDB との連携を使わないので、ドラマの視聴管理の画面も無効にする (#1652)。
    dramaLibrary: () => ({ enabled: false, works: [] }),
    // デモは Simkl に問い合わせないので、見たドラマの記録も無効にする (#1649)。
    dramaRecordApps: () => [
      {
        destination: 'SIMKL',
        clientId: null,
        configured: false,
        redirectUri: `${window.location.origin}/internal/auth/simkl/callback`,
        updatedAt: null,
      },
    ],
    dramaRecordSettings: () => demoDramaRecordSettings(),
    // デモは外部へ問い合わせないので、連携していないものとして出さない。
    animeFollowingViewings: () => ({
      enabled: false,
      needsRefresh: false,
      failed: false,
      viewings: [],
    }),
    storagePoolNames: () => ['default'],
    // デモは外部へ問い合わせないので、字幕の自動生成のモデル (#1682) は置かず、選べるモデルだけを見せる。ダウンロードはしない。
    captionModels: () => demoCaptionModels(),
    // デモは DB を書き出さないので、公開鍵のない既定の設定だけを見せる (#1926)。
    backup: () => ({
      settings: { recipients: [], intervalHours: 24, generations: 7, updatedAt: null },
      latestRun: null,
    }),
    fileNameTemplateHelp: () => ({
      items: fileNameTemplateItems.map((item) => ({
        ...item,
        example: renderFileName(item.snippet, sampleFileNameData).fileName ?? '',
      })),
      presets: fileNameTemplatePresets.map((preset) => ({
        ...preset,
        example: renderFileName(preset.template, sampleFileNameData).fileName ?? '',
      })),
    }),
    previewFileName: ({ template, program: id }: { template: string; program?: string | null }) => {
      const item = id == null ? undefined : programsById.get(id);

      return renderFileName(
        template,
        item === undefined
          ? sampleFileNameData
          : {
              name: item.name,
              serviceName: item.service.name,
              channelType: item.service.channelType.type,
              channel: item.service.channel,
              eventId: item.eventId,
              startAt: item.start,
              endAt: item.end,
            },
      );
    },

    // Mutation
    // デモは DB を書き出さないので、バックアップの設定を変えさせない (#1926)。
    updateBackupSettings: () => {
      throw new Error('backups are not available in the demo');
    },
    runBackup: () => {
      throw new Error('backups are not available in the demo');
    },
    // デモは作品の指定を保持しない (#1651)。
    updateDramaTitleOverride: () => [],
    // デモは Annict と AniList に問い合わせないので、見たいリストを取り込めず、候補もない (#928)。
    refreshAnimeWishlist: () => {
      throw new Error('anime linking is disabled');
    },
    createAnimeWishlistRule: () => {
      throw new Error('the work is not in your wishlist candidates');
    },
    createAnimeFollowingWatchingRule: () => {
      throw new Error('the work is not watched by the users you follow');
    },
    updateAnimeChannel: ({
      serviceId,
      input,
    }: {
      serviceId: string;
      input: {
        syobocalChannelId?: number | null;
        resetSyobocalChannelId: boolean;
        annictChannelId?: number | null;
        resetAnnictChannelId: boolean;
      };
    }) => {
      const current = animeOverrides.get(serviceId) ?? { syobocal: null, annict: null };
      const next = {
        syobocal: input.resetSyobocalChannelId
          ? null
          : (input.syobocalChannelId ?? current.syobocal),
        annict: input.resetAnnictChannelId ? null : (input.annictChannelId ?? current.annict),
      };
      animeOverrides.set(serviceId, next);

      return animeChannel(serviceId);
    },
    deleteCapture: ({ id }: { id: string }) => captures.remove(id),
    // デモでは、見たアニメを記録しない (#927)。プレイヤーの報告は受け取って破棄し、記録の対象でないと答える。
    reportAnimePlayback: () => false,
    updateAnimeRecordSettings: () => demoAnimeRecordSettings(),
    // デモでは Simkl と連携しない (#1649)。ウォッチリストの取り込みも頼まれても入れない。
    importDramaWatchlist: () => false,
    disconnectDramaRecord: () => demoDramaRecordSettings(),
    setRecordingTags: ({ id, tags }: { id: string; tags: string[] }) =>
      recordings.setTags(id, tags),
    setRecordingProtected: ({ id, protected: value }: { id: string; protected: boolean }) =>
      recordings.setProtected(id, value),
    deleteRecording: ({ id }: { id: string }) => recordings.delete(id),
    deleteRecordings: ({ ids }: { ids: string[] }) => recordings.deleteMany(ids),
    // インデックスや画像を作り直すジョブは、デモでは何もせずに受け付ける。
    reanalyzeRecording: () => true,
    // CM を解析するジョブは、待って、進み具合を出して、終える様子を真似る。デモの映像は CM を含まないので、区間は見つからない。
    detectRecordingCm: ({ id }: { id: string }) => recordings.detectCm(id),
    generateRecordingCaptions: ({ id }: { id: string }) => recordings.generateCaptions(id),
    regenerateRecordingImages: () => true,
    saveWatchPosition: ({ id, positionMs }: { id: string; positionMs: number }) =>
      recordings.saveWatchPosition(id, positionMs),
    createStreamURL: ({ target }: { target: string }) => {
      const found = node(target) as { streamUrl?: string; startAt?: string; endAt?: string } | null;
      if (found?.streamUrl === undefined) {
        throw new Error('the target is not found');
      }
      // 期限は、サーバーと同じく、録画なら録画の長さに 1 時間を足した長さ、ライブなら 1 時間にする (#1756)。
      const hour = 60 * 60 * 1000;
      const length =
        found.startAt !== undefined && found.endAt !== undefined
          ? Math.max(Date.parse(found.endAt) - Date.parse(found.startAt), 0)
          : 0;

      return {
        url: found.streamUrl,
        expiresAt: new Date(Date.now() + length + hour).toISOString(),
      };
    },
    updatePlayerSettings: ({
      input,
    }: {
      input: {
        jikkyoEnabled?: boolean | null;
        commentSources?: string[] | null;
        captionsEnabled?: boolean | null;
        comments?: Record<string, unknown> | null;
        captions?: Record<string, unknown> | null;
        preferences?: { key: string; value: string | null }[] | null;
      };
    }) => {
      const preferences = new Map(playerSettings.preferences.map(({ key, value }) => [key, value]));
      for (const { key, value } of input.preferences ?? []) {
        if (value === null) {
          preferences.delete(key);
        } else {
          preferences.set(key, value);
        }
      }
      const nextSources = () => {
        if (input.commentSources != null) {
          return input.commentSources;
        }
        if (input.jikkyoEnabled == null) {
          return playerSettings.commentSources;
        }

        return input.jikkyoEnabled ? ['NX_JIKKYO'] : [];
      };
      playerSettings = {
        jikkyoEnabled: (() => {
          const next = nextSources();

          return next.includes('NX_JIKKYO');
        })(),
        commentSources: nextSources(),
        captionsEnabled: input.captionsEnabled ?? playerSettings.captionsEnabled,
        comments: { ...playerSettings.comments, ...input.comments },
        captions: { ...playerSettings.captions, ...input.captions },
        preferences: [...preferences]
          .map(([key, value]) => ({ key, value }))
          .sort((a, b) => a.key.localeCompare(b.key)),
      };
      viewerSettingsChanged();

      return playerSettings;
    },
    registerPlayer: ({ state }: { state: object }) => {
      const at = new Date().toISOString();

      return {
        id: `Player:${nextPlayer++}`,
        current: true,
        userAgent: navigator.userAgent,
        state,
        startedAt: at,
        lastSeenAt: at,
      };
    },
    updatePlayer: ({ id, state }: { id: string; state: object }) => {
      const at = new Date().toISOString();

      return {
        id,
        current: true,
        userAgent: navigator.userAgent,
        state,
        startedAt: at,
        lastSeenAt: at,
      };
    },
    unregisterPlayer: ({ id }: { id: string }) => id,
    createProgramReservation: ({
      input,
    }: {
      input: {
        program: string;
        followEventRelay?: boolean | null;
        settings?: SettingsInput | null;
      };
    }) => scheduler.createProgramReservation(input),
    createTimeReservation: ({
      input,
    }: {
      input: Parameters<typeof scheduler.createTimeReservation>[0];
    }) => scheduler.createTimeReservation(input),
    updateReservation: ({
      id,
      input,
    }: {
      id: string;
      input: Parameters<typeof scheduler.updateReservation>[1];
    }) => scheduler.updateReservation(id, input),
    deleteReservation: ({ id }: { id: string }) => scheduler.deleteReservation(id),
    deleteReservations: ({ ids }: { ids: string[] }) => scheduler.deleteReservations(ids),
    setReservationsEnabled: ({ ids, enabled }: { ids: string[]; enabled: boolean }) =>
      scheduler.setReservationsEnabled(ids, enabled),
    createRule: ({ input }: { input: RuleInput }) => scheduler.createRule(input),
    updateRule: ({ id, input }: { id: string; input: RuleInput }) =>
      scheduler.updateRule(id, input),
    deleteRule: ({ id }: { id: string }) => scheduler.deleteRule(id),
    deleteRules: ({ ids }: { ids: string[] }) => scheduler.deleteRules(ids),
    setRuleEnabled: ({ id, enabled }: { id: string; enabled: boolean }) => {
      scheduler.setRulesEnabled([id], enabled);

      return scheduler.ruleView(id);
    },
    setRulesEnabled: ({ ids, enabled }: { ids: string[]; enabled: boolean }) =>
      scheduler.setRulesEnabled(ids, enabled),
    setRulesGroup: ({ ids, groupId }: { ids: string[]; groupId?: string | null }) =>
      scheduler.setRulesGroup(ids, groupId ?? null),
    createRuleGroup: ({ name }: { name: string }) => scheduler.createRuleGroup(name),
    renameRuleGroup: ({ id, name }: { id: string; name: string }) =>
      scheduler.renameRuleGroup(id, name),
    reorderRuleGroups: ({ ids }: { ids: string[] }) => scheduler.reorderRuleGroups(ids),
    deleteRuleGroup: ({ id }: { id: string }) => scheduler.deleteRuleGroup(id),
    updateWatchReminderSettings: ({ minutesBefore: value }: { minutesBefore: number }) => {
      minutesBefore = value;
      viewerSettingsChanged();

      return { minutesBefore };
    },
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
    createWatchRule: ({ input }: { input: WatchRuleInput }) => {
      const id = `WatchRule:${nextWatchRule}`;
      nextWatchRule += 1;
      const rule = { input, createdAt: Date.now(), updatedAt: Date.now() };
      watchRules.set(id, rule);
      applyWatchRules();
      viewerSettingsChanged();

      return watchRuleView(id, rule);
    },
    updateWatchRule: ({ id, input }: { id: string; input: WatchRuleInput }) => {
      const found = watchRules.get(id);
      if (found === undefined) {
        throw new Error('the watch rule is not found');
      }
      const rule = { ...found, input, updatedAt: Date.now() };
      watchRules.set(id, rule);
      applyWatchRules();
      viewerSettingsChanged();

      return watchRuleView(id, rule);
    },
    setWatchRuleEnabled: ({ id, enabled }: { id: string; enabled: boolean }) => {
      const found = watchRules.get(id);
      if (found === undefined) {
        throw new Error('the watch rule is not found');
      }
      const rule = { ...found, input: { ...found.input, enabled }, updatedAt: Date.now() };
      watchRules.set(id, rule);
      applyWatchRules();
      viewerSettingsChanged();

      return watchRuleView(id, rule);
    },
    deleteWatchRule: ({ id }: { id: string }) => {
      // 視聴ルールを消したら、付けた視聴予約のうち、放送の前のものも外す (#1268)。
      // 放送が始まった番組のものは、手で付けたものとして残す。
      // ほかの有効な視聴ルールにも一致する番組は、そのルールが付けたものとして扱い続ける。
      watchRules.delete(id);
      const remaining = [...watchRules.values()].filter((rule) => rule.input.enabled !== false);
      for (const programId of watchRuleReserved) {
        const item = programsById.get(programId);
        if (
          item === undefined ||
          !remaining.some((rule) => matchProgram(item, rule.input.conditions))
        ) {
          watchRuleReserved.delete(programId);
          if (item !== undefined && item.start > now) {
            watchReserved.delete(programId);
          }
        }
      }
      viewerSettingsChanged();

      return id;
    },
    setWatchReservation: ({ program: id, reserved }: { program: string; reserved: boolean }) => {
      const found = programsById.get(id);
      if (found === undefined) {
        throw new Error('the program is not found');
      }
      // 放送が始まった番組と、過去の番組には付けられない。
      if (reserved && found.start <= now) {
        throw new Error('the program has already started');
      }
      if (reserved) {
        watchReserved.add(id);
      } else {
        watchReserved.delete(id);
      }
      // 手で付け外しした視聴予約は、視聴ルールが付けたものとして扱わない。
      watchRuleReserved.delete(id);
      viewerSettingsChanged();

      return program(found);
    },

    // Subscription。graphql-js は、流れた値を rootValue にして選択集合を解くので、フィールド名で包む。
    // 本物のサーバーと同じく、購読を始めたら、まず種類ごとの今のバージョンと設定のバージョンを送る (設計書 9.1)。
    // 画面は最初に届いたバージョンを今のバージョンとして保持するだけなので、送らないと、最初の変更で読み直さない。
    viewerEvents: () => {
      // バージョンを読む前に購読し、読んだ後の変更を取りこぼさないようにする。
      const live = events.listen((event) => ({ viewerEvents: event }));
      const quiet = {
        unreadNotificationCount: null,
        streamsDraining: null,
        liveStreamScrambled: null,
      };
      const current = [
        ...demoDataChannels.map((channel) => ({
          viewerEvents: {
            ...quiet,
            dataChange: { channel, version: dataVersions.get(channel) ?? 0 },
            viewerSettingsVersion: null,
          },
        })),
        { viewerEvents: { ...quiet, dataChange: null, viewerSettingsVersion } },
      ];
      // ジェネレーターで書くと、待っている next を return で終わらせられない (events.ts の listen と同じ)。
      // そこで、今のバージョンを送り終えたら、listen のイテレーターにそのまま任せる。
      const iterator: AsyncIterableIterator<(typeof current)[number] | { viewerEvents: unknown }> =
        {
          next: async () => {
            const value = current.shift();

            return value === undefined ? live.next() : { done: false, value };
          },
          return: async () => {
            current.length = 0;

            return live.return === undefined ? { done: true, value: undefined } : live.return();
          },
          [Symbol.asyncIterator]: () => iterator,
        };

      return iterator;
    },
    playerCommands: () => idle.listen(() => ({ playerCommands: null })),
  };
}

// demoCaptionModels は、デモで見せる、字幕の自動生成のモデルの置き場所 (#1682)。どれも置いていない。
function demoCaptionModels() {
  const model = (
    id: string,
    kind: 'WHISPER' | 'VAD',
    name: string,
    size: number,
    license: string,
    recommended: boolean,
  ) => ({ id, kind, name, size, sha256: '', license, url: '', recommended });

  return [
    {
      kind: 'WHISPER',
      state: 'NOT_PLACED',
      overridden: false,
      installed: null,
      download: null,
      models: [
        model(
          'kotoba-whisper-v2.0-q5_0',
          'WHISPER',
          'kotoba-whisper v2.0 (q5_0)',
          537819875,
          'Apache-2.0',
          true,
        ),
        model('whisper-small-q5_1', 'WHISPER', 'Whisper small (q5_1)', 190085487, 'MIT', false),
        model(
          'whisper-large-v3-turbo-q5_0',
          'WHISPER',
          'Whisper large-v3-turbo (q5_0)',
          574041195,
          'MIT',
          false,
        ),
      ],
    },
    {
      kind: 'VAD',
      state: 'NOT_PLACED',
      overridden: false,
      installed: null,
      download: null,
      models: [model('silero-v5.1.2', 'VAD', 'Silero VAD v5.1.2', 885098, 'MIT', true)],
    },
  ];
}
