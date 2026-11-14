import { demoAnimeEpisodes } from './anime';
import { demoStreamURL, type DemoProgram } from './epg';
import { demoCommentsPath } from './jikkyo';

// デモの録画 (#857)。放送が終わった番組から、ルールに当てはまるものなどを録画済みとして置く。
// 録画のファイルはなく、映像は、どの録画もデモの映像 (demoStreamURL) を流す。

// RecordingRule は、録画を作ったルール。録画の最初の状態を作るときに、番組がどのルールに当てはまるかを見る。
export type RecordingRule = {
  id: string;
  name: string;
  matches: (program: DemoProgram) => boolean;
};

type Recording = {
  id: string;
  program: DemoProgram;
  state: 'RECORDING' | 'COMPLETED' | 'COMPLETED_WITH_ERRORS';
  protected: boolean;
  drops: number;
  tags: string[];
  watchPositionMs: number | null;
  watchedAt: number;
  rule: RecordingRule | undefined;
  // cm は、CM の区間の見分け (#1571)。requestedAt は、見分けを頼んだ時刻。見分け終えたら null に戻し、detectedAt に終えた時刻を書く。
  cm: { requestedAt: number | null; detectedAt: number | null };
};

// cmQueuedMs と cmRunningMs は、デモで CM の区間の見分けを待つ時間と、見分ける時間 (#1571)。
// 本物は、30 分の録画で 1 分ほどかかる。デモでは、進み具合が動く様子を見られる程度に縮める。
const cmQueuedMs = 3_000;
const cmRunningMs = 15_000;

export type RecordingFilter = {
  tagIds?: string[] | null;
  states?: string[] | null;
  serviceIds?: string[] | null;
  keyword?: string | null;
  since?: string | null;
  until?: string | null;
  ruleId?: string | null;
  genres?: number[] | null;
};

const tagId = (name: string) => `Tag:${encodeURIComponent(name)}`;

// thumbnail は、番組名とジャンルの色で描いた、16:9 の代表画像を data: URL で返す。
function thumbnail(program: DemoProgram): string {
  const colors = ['#4dabf7', '#69db7c', '#ffd43b', '#f783ac', '#b197fc', '#ffa94d', '#63e6be'];
  const color = colors[(program.genres[0]?.lv1 ?? 0) % colors.length]!;
  const name = program.name.replace(/[&<>"]/gu, '');
  const svg =
    `<svg xmlns="http://www.w3.org/2000/svg" width="320" height="180" viewBox="0 0 320 180">` +
    `<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="${color}"/>` +
    `<stop offset="1" stop-color="#343a40"/></linearGradient></defs>` +
    `<rect width="320" height="180" fill="url(#g)"/>` +
    `<text x="160" y="96" font-family="sans-serif" font-size="16" fill="#fff" text-anchor="middle">${name}</text>` +
    `<text x="160" y="124" font-family="sans-serif" font-size="12" fill="#fff" fill-opacity="0.8" text-anchor="middle">${program.service.name}</text>` +
    `</svg>`;

  return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`;
}

export function createRecordings(options: {
  programs: DemoProgram[];
  now: number;
  rules: RecordingRule[];
  // ruleView は、録画のルールを返す。ルールを消していれば null。
  ruleView: (id: string) => object | null;
  // changed は、録画が変わったことを画面に知らせる。CM の区間を見分け終えたときに使う。
  changed?: (channels: string[]) => void;
}) {
  const { programs, now, rules, ruleView, changed } = options;
  const recordings = new Map<string, Recording>();

  const ruleOf = (program: DemoProgram) => rules.find((rule) => rule.matches(program));
  const add = (program: DemoProgram, fields: Partial<Recording> = {}) => {
    const id = `Recording:${program.id.slice('Program:'.length)}`;
    const rule = ruleOf(program);
    recordings.set(id, {
      id,
      program,
      state: program.end > now ? 'RECORDING' : 'COMPLETED',
      protected: false,
      drops: 0,
      tags: [],
      watchPositionMs: null,
      watchedAt: 0,
      rule,
      cm: { requestedAt: null, detectedAt: null },
      ...fields,
    });
  };

  // 最初の状態。ルールに当てはまる、放送が終わった番組と、いくつかの映画やドキュメントを録画済みにする。
  const past = programs.filter(
    (item) => item.end <= now && item.end > now - 3 * 24 * 60 * 60 * 1000,
  );
  for (const program of past) {
    if (ruleOf(program) !== undefined) {
      const lv1 = program.genres[0]?.lv1;
      add(program, { tags: lv1 === 7 ? ['アニメ'] : lv1 === 3 ? ['ドラマ'] : [] });
    }
  }
  for (const program of past.filter((item) => /^映画|^ドキュメント/u.test(item.name)).slice(0, 4)) {
    add(program, { tags: ['あとで見る'] });
  }
  // 途中まで見た録画、ドロップのあった録画、保護した録画を 1 つずつ作る。
  const finished = [...recordings.values()].toSorted((a, b) => b.program.start - a.program.start);
  if (finished[0] !== undefined) {
    finished[0].watchPositionMs = Math.floor(
      (finished[0].program.end - finished[0].program.start) * 0.4,
    );
    finished[0].watchedAt = now - 60 * 60 * 1000;
  }
  if (finished[1] !== undefined) {
    finished[1].state = 'COMPLETED_WITH_ERRORS';
    finished[1].drops = 37;
  }
  if (finished[2] !== undefined) {
    finished[2].protected = true;
  }
  // 放送中の番組を 1 つ、録画中にする。ドキュメントがなければ、ほかの番組にする。
  const airing = programs.filter((item) => item.start <= now && item.end > now);
  const onAir = airing.find((item) => item.genres[0]?.lv1 === 8) ?? airing[0];
  if (onAir !== undefined) {
    add(onAir);
  }

  // cmDetection は、録画の CM の区間の見分けの状態を、頼んでからの時間で決める (#1571)。
  const cmDetection = (recording: Recording) => {
    const { requestedAt } = recording.cm;
    if (requestedAt !== null) {
      const elapsed = Date.now() - requestedAt;
      const previous =
        recording.cm.detectedAt === null ? null : new Date(recording.cm.detectedAt).toISOString();
      if (elapsed < cmQueuedMs) {
        return { state: 'QUEUED', detectedAt: previous, progress: null };
      }
      if (elapsed < cmQueuedMs + cmRunningMs) {
        const running = elapsed - cmQueuedMs;

        return {
          state: 'RUNNING',
          detectedAt: previous,
          progress: {
            ratio: Math.min(0.99, running / cmRunningMs),
            // 始まってすぐは、本物と同じく、残り時間を計算しているところとする。
            remainingSeconds: running < 2_000 ? null : Math.ceil((cmRunningMs - running) / 1000),
            updatedAt: new Date().toISOString(),
          },
        };
      }
      recording.cm = { requestedAt: null, detectedAt: requestedAt + cmQueuedMs + cmRunningMs };
    }

    return recording.cm.detectedAt === null
      ? { state: 'NOT_DETECTED', detectedAt: null, progress: null }
      : {
          state: 'DETECTED',
          detectedAt: new Date(recording.cm.detectedAt).toISOString(),
          progress: null,
        };
  };

  const view = (recording: Recording) => {
    const { program } = recording;
    const recordedUntil =
      recording.state === 'RECORDING' ? null : new Date(program.end).toISOString();
    const packets = Math.floor((Math.min(program.end, now) - program.start) / 1000) * 1000;

    return {
      __typename: 'Recording',
      id: recording.id,
      service: program.service,
      eventId: program.eventId,
      name: program.name,
      description: program.description,
      extended: program.extended,
      startAt: program.startAt,
      endAt: program.endAt,
      state: recording.state,
      failureReason: null,
      failureDetail: null,
      protected: recording.protected,
      drops: recording.drops,
      errors: 0,
      scrambled: 0,
      // デモの録画は、どれも復号できている。
      scrambleWarning: false,
      scrambledRatio: 0,
      streamQuality: [
        { pid: 0x100, kind: 'VIDEO', packets, drops: recording.drops, errors: 0, scrambled: 0 },
        {
          pid: 0x110,
          kind: 'AUDIO',
          packets: Math.floor(packets / 10),
          drops: 0,
          errors: 0,
          scrambled: 0,
        },
      ],
      gaps: [],
      // デモの録画の映像は CM を含まないので、CM の区間はない。
      cmSections: [],
      cmDetection: () => cmDetection(recording),
      audios: program.audios,
      videoFormat: program.service.videoFormat,
      genres: program.genres,
      workTitle: program.workTitle,
      animeEpisodes: demoAnimeEpisodes(program),
      rule: () => (recording.rule === undefined ? null : ruleView(recording.rule.id)),
      // ruleName は、ルールを消しても残る、録画したときのルールの名前。
      ruleName: recording.rule?.name ?? null,
      programLost: false,
      broadcastStartedAt: program.startAt,
      broadcastEndedAt: recordedUntil,
      notBroadcast: false,
      recordedFrom: program.startAt,
      recordedUntil,
      tags: recording.tags.map((name) => ({ id: tagId(name), name, recordings: count(name) })),
      files: [
        {
          kind: 'SEGMENT',
          poolId: 'default',
          path: `${program.service.name}/${program.name}.m2ts`,
          size: packets * 188 * 8,
          startedAt: program.startAt,
          endedAt: recordedUntil,
          url: null,
        },
      ],
      watchPositionMs: recording.watchPositionMs,
      thumbnailUrl: thumbnail(program),
      previewUrl: null,
      watchFrame: null,
      // 実況の過去ログは、src/demo/index.ts が架空のコメントを返す (src/demo/jikkyo.ts)。
      jikkyoCommentsUrl: demoCommentsPath,
      jikkyoOtherCommentsUrl: `${demoCommentsPath}.others`,
      jikkyoNicologCommentsUrl: `${demoCommentsPath}.nicolog`,
      // 録画の長さを、映像を繰り返して流す Service Worker (demo/stream-sw.js) に伝え、録画の終わりで流し終えさせる。
      // 録画中の録画は、終わりが決まっていないので、終わらずに流す。
      streamUrl:
        recordedUntil === null
          ? demoStreamURL
          : `${demoStreamURL}?length=${(program.end - program.start) / 1000}`,
    };
  };

  const count = (name: string) =>
    [...recordings.values()].filter((item) => item.tags.includes(name)).length;
  const tagNames = () =>
    [...new Set([...recordings.values()].flatMap((item) => item.tags))].toSorted();

  const matches = (recording: Recording, filter: RecordingFilter | null | undefined) => {
    if (filter == null) {
      return true;
    }
    const { program } = recording;
    const keyword = filter.keyword?.trim().toLowerCase();

    return (
      (filter.tagIds == null ||
        filter.tagIds.every((id) => recording.tags.some((name) => tagId(name) === id))) &&
      (filter.states == null || filter.states.includes(recording.state)) &&
      (filter.serviceIds == null || filter.serviceIds.includes(program.service.id)) &&
      (keyword == null || keyword === '' || program.name.toLowerCase().includes(keyword)) &&
      (filter.since == null || program.start >= Date.parse(filter.since)) &&
      (filter.until == null || program.start < Date.parse(filter.until)) &&
      (filter.ruleId == null || recording.rule?.id === filter.ruleId) &&
      (filter.genres == null || program.genres.some((genre) => filter.genres!.includes(genre.lv1)))
    );
  };

  const recordingOrThrow = (id: string) => {
    const recording = recordings.get(id);
    if (recording === undefined) {
      throw new Error('the recording is not found');
    }

    return recording;
  };

  // removable は、消せる録画か。保護した録画と、録画中の録画は消せない。
  const removable = (recording: Recording) =>
    !recording.protected && recording.state !== 'RECORDING';

  return {
    node: (id: string) => {
      const recording = recordings.get(id);

      return recording === undefined ? undefined : view(recording);
    },
    list: (filter: RecordingFilter | null | undefined, order: string) =>
      [...recordings.values()]
        .filter((item) => matches(item, filter))
        .toSorted((a, b) =>
          order === 'OLDEST_FIRST'
            ? a.program.start - b.program.start
            : b.program.start - a.program.start,
        )
        .map(view),
    // continueWatching は、途中まで見た録画を、最後に見た新しい順に返す。
    // 10 秒より前まで見た録画は含めない。終わりの 10 秒以内か、長さの 90% 以上まで見た録画は、見終えたものとして含めない (#1301)。
    continueWatching: (first: number) =>
      [...recordings.values()]
        .filter((item) => {
          const duration = item.program.end - item.program.start;

          return (
            item.watchPositionMs !== null &&
            item.watchPositionMs >= 10_000 &&
            item.watchPositionMs < Math.min(duration - 10_000, duration * 0.9)
          );
        })
        .toSorted((a, b) => b.watchedAt - a.watchedAt)
        .slice(0, first)
        .map(view),
    tags: () => tagNames().map((name) => ({ id: tagId(name), name, recordings: count(name) })),
    setTags: (id: string, tags: string[]) => {
      const recording = recordingOrThrow(id);
      recording.tags = [...new Set(tags.map((name) => name.trim()).filter((name) => name !== ''))];

      return view(recording);
    },
    setProtected: (id: string, value: boolean) => {
      const recording = recordingOrThrow(id);
      recording.protected = value;

      return view(recording);
    },
    delete: (id: string) => {
      const recording = recordingOrThrow(id);
      if (!removable(recording)) {
        throw new Error('the recording is protected or still recording');
      }
      recordings.delete(id);

      return id;
    },
    deleteMany: (ids: string[]) => {
      let processed = 0;
      for (const id of ids) {
        const recording = recordings.get(id);
        if (recording !== undefined && removable(recording)) {
          recordings.delete(id);
          processed += 1;
        }
      }

      return { processed, skipped: ids.length - processed };
    },
    // detectCm は、CM の区間の見分けを頼む (#1571)。見分けを待っているか見分けている間は、重ねて頼まずに false を返す。
    // 本物と同じく、頼んだときと見分け終えたときに、録画が変わったことを知らせる。
    detectCm: (id: string) => {
      const recording = recordingOrThrow(id);
      if (recording.state === 'RECORDING') {
        throw new Error('the recording is still being recorded');
      }
      if (cmDetection(recording).state === 'QUEUED' || cmDetection(recording).state === 'RUNNING') {
        return false;
      }
      recording.cm = { ...recording.cm, requestedAt: Date.now() };
      changed?.(['RECORDINGS']);
      setTimeout(() => changed?.(['RECORDINGS']), cmQueuedMs + cmRunningMs);

      return true;
    },
    saveWatchPosition: (id: string, positionMs: number) => {
      const recording = recordingOrThrow(id);
      recording.watchPositionMs = positionMs;
      recording.watchedAt = Date.now();

      return view(recording);
    },
  };
}
