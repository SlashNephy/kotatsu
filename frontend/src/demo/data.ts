import { demoAnimeEpisodes } from './anime';
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

type WatchRuleInput = {
  name: string;
  enabled?: boolean | null;
  conditions: ProgramConditionsInput;
};

type WatchRule = { input: WatchRuleInput; createdAt: number; updatedAt: number };

const genreCondition = (genre: GenreInput) => ({ lv1: genre.lv1, lv2: genre.lv2 ?? null });

const servicesOf = (ids: readonly string[]) =>
  ids.flatMap((id) => services.filter((item) => item.id === id));

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
  // dataChanged は、予約やルールを変えたことを、変わった種類ごとに版を上げて画面に知らせる。
  const dataChanged = (channels: string[]) => {
    for (const channel of channels) {
      const version = (dataVersions.get(channel) ?? 0) + 1;
      dataVersions.set(channel, version);
      events.publish({
        dataChange: { channel, version },
        viewerSettingsVersion: null,
        unreadNotificationCount: null,
        streamsDraining: null,
      });
    }
  };
  const scheduler = createScheduler({ programs, now, user: demoUser, changed: dataChanged });
  const recordings = createRecordings({
    programs,
    now,
    rules: scheduler.recordingRules(),
    ruleView: scheduler.findRule,
  });
  let playerSettings = {
    jikkyoEnabled: false,
    captionsEnabled: true,
    comments: { opacity: 1, speed: 1, scale: 1, showAll: false, mutedKeywords: [], mutedUsers: [] },
    captions: {
      superimposeLive: true,
      superimposeRecorded: true,
      stroke: true,
      backgroundOpacity: null,
    },
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
    animeEpisodes: demoAnimeEpisodes(item),
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
  // 視聴ルールが付けた視聴予約のうち、一致しなくなったものを外す。手で外した番組を覚えることは、デモでは省く。
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

  // node は、node(id) で引く。ID の頭に型の名前を付けて、型をまたいで重ならないようにしている。
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

  // viewerSettingsChanged は、ピン留め、番組表ビュー、視聴予約を変えたことを、画面に知らせる。
  const viewerSettingsChanged = () => {
    viewerSettingsVersion += 1;
    events.publish({
      dataChange: null,
      viewerSettingsVersion,
      unreadNotificationCount: null,
      streamsDraining: null,
    });
  };

  return {
    // Query
    viewer: () => demoUser,
    captures: (args: { keyword?: string | null }) => captures.list(args),
    capturesAvailable: () => true,
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
    systemSettings: () => ({
      recordingDefaults: {
        poolId: 'default',
        fileNameTemplate: defaultFileNameTemplate,
        startMarginSeconds: 0,
        endMarginSeconds: 0,
        priority: 0,
        joinSegments: true,
      },
      archiveRetentionDays: 30,
      notificationRetentionDays: 30,
      ruleDefaults: { avoidDuplicates: false, duplicateWindowDays: null, followEventRelay: true },
      // デモには tenban がないので、データ放送の文書の通信は中継しない (#501)。
      dataBroadcastIpEnabled: false,
      // デモは外部へ問い合わせないので、しょぼいカレンダー、Annict、AniList との連携は使わない (設計書 3.8)。
      animeLinkingEnabled: false,
      updatedAt: null,
    }),
    // デモには、saya-definitions から取り込んだ対応がない。管理者の上書きだけを覚える。
    animeChannels: () => services.map((service) => animeChannel(service.id)),
    animeImports: () => [],
    storagePoolNames: () => ['default'],
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
    setRecordingTags: ({ id, tags }: { id: string; tags: string[] }) =>
      recordings.setTags(id, tags),
    setRecordingProtected: ({ id, protected: value }: { id: string; protected: boolean }) =>
      recordings.setProtected(id, value),
    deleteRecording: ({ id }: { id: string }) => recordings.delete(id),
    deleteRecordings: ({ ids }: { ids: string[] }) => recordings.deleteMany(ids),
    // 索引や画像を作り直すジョブは、デモでは何もせずに受け付ける。
    reanalyzeRecording: () => true,
    regenerateRecordingImages: () => true,
    saveWatchPosition: ({ id, positionMs }: { id: string; positionMs: number }) =>
      recordings.saveWatchPosition(id, positionMs),
    createStreamURL: ({ target }: { target: string }) => {
      const found = node(target) as { streamUrl?: string } | null;
      if (found?.streamUrl === undefined) {
        throw new Error('the target is not found');
      }

      return {
        url: found.streamUrl,
        expiresAt: new Date(Date.now() + 6 * 60 * 60 * 1000).toISOString(),
      };
    },
    updatePlayerSettings: ({
      input,
    }: {
      input: {
        jikkyoEnabled?: boolean | null;
        captionsEnabled?: boolean | null;
        comments?: Record<string, unknown> | null;
        captions?: Record<string, unknown> | null;
      };
    }) => {
      playerSettings = {
        jikkyoEnabled: input.jikkyoEnabled ?? playerSettings.jikkyoEnabled,
        captionsEnabled: input.captionsEnabled ?? playerSettings.captionsEnabled,
        comments: { ...playerSettings.comments, ...input.comments },
        captions: { ...playerSettings.captions, ...input.captions },
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
      // 視聴ルールを消しても、付けた視聴予約は、手で付けたものとして残す。
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
    viewerEvents: () => events.listen((event) => ({ viewerEvents: event })),
    playerCommands: () => idle.listen(() => ({ playerCommands: null })),
  };
}
