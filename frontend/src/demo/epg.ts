// デモの局と番組 (#832)。局名と番組名は、すべて架空のもの。
// 番組は、開いた時刻を中心に、過去 3 日から先 8 日までを作る。乱数の種を日付から決めるので、同じ日に開けば同じ番組表になる。

export type Genre = { lv1: number; lv2: number };

export type DemoService = {
  __typename: 'Service';
  id: string;
  networkId: number;
  serviceId: number;
  name: string;
  channelType: { type: string; displayName: string; sortOrder: number };
  channel: string;
  remoteControlKeyId: number | null;
  hidden: boolean;
  sortOrder: number | null;
  streamUrl: string;
  logoUrl: string | null;
  jikkyo: null;
  // videoFormat は、放送中の番組の映像の種類。デモの局は、どれも 2K にする。
  videoFormat: 'HD';
};

export type DemoProgram = {
  __typename: 'Program';
  id: string;
  service: DemoService;
  eventId: number;
  startAt: string;
  endAt: string;
  endUndecided: boolean;
  isFree: boolean;
  name: string;
  description: string;
  extended: { heading: string; text: string }[];
  genres: Genre[];
  audios: { componentType: number; isMain: boolean; languages: string[]; samplingRate: number }[];
  sharedFrom: null;
  // start と end は、照合や並べ替えに使う、ミリ秒の時刻。
  start: number;
  end: number;
};

export const channelTypes = [
  { type: 'GR', displayName: '地上波', sortOrder: 0 },
  { type: 'BS', displayName: 'BS', sortOrder: 1 },
];

// demoStreamURL は、デモの映像 (20 秒の合成の映像) の URL。ライブ視聴と録画の再生で、どの局と録画もこれを流す。
export const demoStreamURL = `${import.meta.env.BASE_URL}demo/stream.m2ts`;

// logo は、局の頭文字を色の付いた四角に載せた、64×36 の SVG を data: URL で返す。
function logo(text: string, color: string): string {
  const svg =
    `<svg xmlns="http://www.w3.org/2000/svg" width="64" height="36" viewBox="0 0 64 36">` +
    `<rect width="64" height="36" rx="6" fill="${color}"/>` +
    `<text x="32" y="25" font-family="sans-serif" font-size="16" font-weight="bold" fill="#fff" text-anchor="middle">${text}</text>` +
    `</svg>`;

  return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`;
}

type ServiceSeed = {
  type: 'GR' | 'BS';
  serviceId: number;
  name: string;
  short: string;
  color: string;
  channel: string;
  key: number | null;
  // lineup は、この局が流す番組の傾向。genre の番号の並び。
  lineup: number[];
};

const serviceSeeds: ServiceSeed[] = [
  {
    type: 'GR',
    serviceId: 1024,
    name: 'こたつテレビ',
    short: 'こ',
    color: '#e8590c',
    channel: '27',
    key: 1,
    lineup: [0, 2, 3, 5, 0, 8],
  },
  {
    type: 'GR',
    serviceId: 1032,
    name: 'みかん放送',
    short: 'み',
    color: '#f08c00',
    channel: '26',
    key: 2,
    lineup: [10, 7, 8, 10, 0, 4],
  },
  {
    type: 'GR',
    serviceId: 1040,
    name: 'ぬくぬくテレビ',
    short: 'ぬ',
    color: '#1c7ed6',
    channel: '25',
    key: 4,
    lineup: [2, 5, 3, 7, 1, 0],
  },
  {
    type: 'GR',
    serviceId: 1048,
    name: 'はんてん放送',
    short: 'は',
    color: '#2f9e44',
    channel: '22',
    key: 5,
    lineup: [2, 5, 6, 3, 0, 4],
  },
  {
    type: 'GR',
    serviceId: 1056,
    name: 'どてらテレビ',
    short: 'ど',
    color: '#c2255c',
    channel: '21',
    key: 6,
    lineup: [5, 2, 7, 3, 1, 0],
  },
  {
    type: 'GR',
    serviceId: 1064,
    name: 'ゆず放送',
    short: 'ゆ',
    color: '#7048e8',
    channel: '23',
    key: 7,
    lineup: [5, 3, 2, 7, 6, 0],
  },
  {
    type: 'BS',
    serviceId: 101,
    name: 'BSこたつ',
    short: 'BSこ',
    color: '#d9480f',
    channel: 'BS15_0',
    key: 1,
    lineup: [8, 4, 6, 10, 0],
  },
  {
    type: 'BS',
    serviceId: 141,
    name: 'BSみかん',
    short: 'BSみ',
    color: '#e67700',
    channel: 'BS13_0',
    key: 4,
    lineup: [10, 8, 3, 2, 4],
  },
  {
    type: 'BS',
    serviceId: 181,
    name: 'BSねこ',
    short: 'BSね',
    color: '#5f3dc4',
    channel: 'BS13_1',
    key: 8,
    lineup: [7, 6, 8, 3, 10],
  },
];

export const services: DemoService[] = serviceSeeds.map((seed, index) => {
  const channelType = channelTypes.find((type) => type.type === seed.type) ?? channelTypes[0]!;

  return {
    __typename: 'Service',
    id: `Service:${seed.serviceId}`,
    networkId: seed.type === 'GR' ? 32736 : 4,
    serviceId: seed.serviceId,
    name: seed.name,
    channelType,
    channel: seed.channel,
    remoteControlKeyId: seed.key,
    hidden: false,
    sortOrder: index,
    streamUrl: demoStreamURL,
    logoUrl: logo(seed.short, seed.color),
    // 実況は、外部の NX-Jikkyo につながるので、デモでは持たせない。
    jikkyo: null,
    videoFormat: 'HD',
  };
});

// hours は、放送する時間帯 (日本時間の時、[始まり, 終わり))。24 以上は、翌日の深夜を表す。
// channels は、この番組を流す局 (serviceId)。省くと、どの局でも流す。
// repeats が true の番組は、1 日に何度でも流す。そうでない番組は、局ごとに 1 日 1 回までにする。
type Template = {
  name: (episode: number) => string;
  hours: [number, number];
  channels?: number[];
  repeats?: boolean;
  minutes: number[];
  description: string;
  lv2: number;
};

// templates は、ジャンル (ARIB の大分類) ごとの番組の雛形。
const templates: Record<number, Template[]> = {
  0: [
    {
      name: () => 'おひるのニュース',
      hours: [11, 13],
      minutes: [15, 30],
      description: '午前の出来事と、午後のお天気をお伝えします。',
      lv2: 0,
    },
    {
      name: () => 'こたつニュース',
      repeats: true,
      hours: [5, 9],
      minutes: [30, 60],
      description: '今日の出来事を、こたつに入ったままお伝えします。',
      lv2: 0,
    },
    {
      name: () => 'よるのこたつニュース',
      repeats: true,
      hours: [21, 24],
      minutes: [30, 60],
      description: '一日の出来事を、こたつでゆっくり振り返ります。',
      lv2: 0,
    },
    {
      name: () => 'ニュースとお天気',
      repeats: true,
      hours: [17, 19],
      minutes: [15, 30],
      description: '各地のお天気と、交通の情報をお伝えします。',
      lv2: 1,
    },
  ],
  1: [
    {
      name: () => 'こたつ相撲 本場所',
      hours: [15, 18],
      minutes: [60, 120],
      description: 'こたつの天板を土俵に見立てた、架空の相撲の本場所をお届けします。',
      lv2: 0,
    },
    {
      name: (n) => `週末ボウリング中継 第${n}戦`,
      channels: [1040, 1056],
      hours: [13, 23],
      minutes: [90, 120],
      description: '架空のリーグの熱戦を、レーンの脇からお届けします。',
      lv2: 9,
    },
    {
      name: () => 'スポーツダイジェスト',
      hours: [22, 26],
      minutes: [30],
      description: '今週の試合の見どころを振り返ります。',
      lv2: 0,
    },
  ],
  2: [
    {
      name: () => 'ゆうがたワイド ぬくぬく',
      hours: [15, 18],
      minutes: [60, 120],
      description: '帰り道に役立つ、近所の話題と晩ごはんのヒント。',
      lv2: 4,
    },
    {
      name: () => 'ゆずの朝',
      hours: [5, 9],
      minutes: [60, 90],
      description: '目覚めの一杯と、今日の暮らしの予定をお届けします。',
      lv2: 4,
    },
    {
      name: () => 'あさのぬくもり',
      hours: [5, 10],
      minutes: [60, 120],
      description: '暮らしの話題と、季節の便りをお届けする情報番組。',
      lv2: 4,
    },
    {
      name: () => 'ひるまえ情報局',
      hours: [10, 14],
      minutes: [60, 90],
      description: '旬の食材と、近所のお店を紹介します。',
      lv2: 3,
    },
  ],
  3: [
    {
      name: (n) => `ドラマ再放送「湯けむり家族」 第${n}話`,
      hours: [13, 17],
      minutes: [60],
      description: '温泉宿を営む家族の、にぎやかな毎日を描いたドラマの再放送。',
      lv2: 0,
    },
    {
      name: (n) => `連続ドラマ「縁側の午後」 第${n}話`,
      channels: [1024],
      hours: [19, 24],
      minutes: [60],
      description: '古い一軒家に越してきた家族と、近所の人たちの物語。',
      lv2: 0,
    },
    {
      name: (n) => `ドラマ「みかん箱の手紙」 第${n}話`,
      channels: [1064, 141],
      hours: [20, 24],
      minutes: [60],
      description: '押し入れから出てきた手紙をめぐる、ある冬の出来事。',
      lv2: 0,
    },
  ],
  4: [
    {
      name: () => 'ミュージックこたつ',
      hours: [23, 26],
      minutes: [30],
      description: '深夜に聴きたい、静かな曲を集めました。',
      lv2: 0,
    },
    {
      name: () => 'うたのこたつ',
      hours: [19, 23],
      minutes: [30, 60],
      description: 'こたつを囲んで、懐かしい歌を歌います。',
      lv2: 0,
    },
  ],
  5: [
    {
      name: () => 'こたつ会議',
      hours: [21, 24],
      minutes: [60],
      description: '芸人たちが、こたつに入ったまま世の中の小さな疑問を話し合います。',
      lv2: 1,
    },
    {
      name: () => 'お笑いみかん劇場',
      hours: [22, 25],
      minutes: [30, 60],
      description: '若手芸人のネタを、みかんを食べながら楽しむ番組。',
      lv2: 0,
    },
    {
      name: (n) => `どてら散歩 #${n}`,
      hours: [12, 17],
      minutes: [60],
      description: 'どてら姿で、下町の商店街をぶらりと歩きます。',
      lv2: 0,
    },
    {
      name: () => 'クイズ！ぬくぬく王',
      channels: [1048, 1056],
      hours: [19, 23],
      minutes: [60],
      description: '冬の暮らしの知恵を競う、こたつ早押しクイズ。',
      lv2: 1,
    },
    {
      name: (n) => `はんてん旅 #${n}`,
      hours: [13, 23],
      minutes: [30, 60],
      description: 'はんてんを着た二人が、あてもなく町を歩きます。',
      lv2: 0,
    },
  ],
  6: [
    {
      name: () => '映画「はんてんの約束」',
      hours: [13, 26],
      minutes: [120],
      description: '祖母が縫ったはんてんに隠された、ある約束をめぐる物語。',
      lv2: 0,
    },
    {
      name: () => '映画「雪の日の湯たんぽ」',
      hours: [13, 26],
      minutes: [120],
      description: '雪に閉じ込められた山小屋で、湯たんぽを分け合う人々の一夜。',
      lv2: 2,
    },
  ],
  7: [
    {
      name: (n) => `アニメ「みかんの国」 #${n}`,
      hours: [17, 19],
      minutes: [30],
      description: 'みかんの国の王子が、冬の祭りを取り戻す冒険に出ます。',
      lv2: 0,
    },
    {
      name: (n) => `アニメ「ねこと湯たんぽ」 #${n}`,
      channels: [1040, 181],
      hours: [16, 19],
      minutes: [30],
      description: 'ねこと湯たんぽが、冬の家を冒険します。',
      lv2: 0,
    },
    {
      name: (n) => `アニメ「こたつ探偵」 第${n}話`,
      channels: [1056, 181],
      hours: [23, 28],
      minutes: [30],
      description: 'こたつから一歩も出ない探偵が、近所の謎を解きます。',
      lv2: 0,
    },
  ],
  8: [
    {
      name: () => 'ねこの町を歩く',
      hours: [19, 23],
      minutes: [30, 60],
      description: 'ねこがたくさん暮らす港町を、一日かけて訪ねます。',
      lv2: 0,
    },
    {
      name: () => 'ドキュメント 炭火の匠',
      hours: [10, 28],
      minutes: [45, 60],
      description: '炭を焼き続けて五十年。山あいの炭焼き小屋の一年を追います。',
      lv2: 1,
    },
    {
      name: () => 'みかん山の四季',

      hours: [4, 28],
      minutes: [30],
      description: '段々畑のみかん山を、季節ごとに訪ねます。',
      lv2: 2,
    },
  ],
  10: [
    {
      name: () => 'こども工作 段ボールのこたつ',
      hours: [16, 18],
      minutes: [15, 30],
      description: '段ボールで、小さなこたつを作ってみます。',
      lv2: 0,
    },
    {
      name: (n) => `こたつで英会話 Lesson ${n}`,
      hours: [10, 13],
      minutes: [15, 30],
      description: 'こたつで使える英語のひとことを練習します。',
      lv2: 0,
    },
    {
      name: (n) => `きょうのお鍋 その${n}`,
      hours: [9, 18],
      minutes: [15, 30],
      description: '冷蔵庫にあるもので作れる、簡単なお鍋を紹介します。',
      lv2: 0,
    },
    {
      name: () => 'はじめての編み物',
      hours: [9, 18],
      minutes: [30],
      description: '靴下を一足、最後まで編み上げます。',
      lv2: 2,
    },
  ],
};

// filler は、ほかに流せる番組がないときの埋め草 (ジャンルは情報/ワイドショーのショッピング)。
const filler: Template = {
  name: () => 'こたつショッピング',
  hours: [0, 28],
  repeats: true,
  minutes: [30, 60],
  description: '冬を暖かく過ごす品々を、電話一本でお届けします。',
  lv2: 15,
};

// mulberry32 は、種から決まる乱数を返す。
function mulberry32(seed: number): () => number {
  let state = seed >>> 0;

  return () => {
    state = (state + 0x6d2b79f5) >>> 0;
    let t = state;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);

    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function pick<T>(random: () => number, items: readonly T[]): T {
  return items[Math.floor(random() * items.length)]!;
}

const minute = 60 * 1000;
const day = 24 * 60 * minute;
// jstOffset は、日本時間の UTC からのずれ。番組表は日本時間の 4 時から始まる日で組む。
const jstOffset = 9 * 60 * minute;

// generatePrograms は、now を含む日の前後の番組を、局ごとに隙間なく並べて作る。
export function generatePrograms(now: number): DemoProgram[] {
  // today は、now を含む放送日 (日本時間の 4 時から翌日の 4 時まで) の始まり。
  const dayStart = jstOffset - 4 * 60 * minute;
  const today = Math.floor((now + dayStart) / day) * day - dayStart;
  const from = today - 3 * day;
  const to = today + 8 * day;
  const programs: DemoProgram[] = [];

  for (const service of services) {
    const seed = serviceSeeds.find((item) => `Service:${item.serviceId}` === service.id)!;
    const random = mulberry32(seed.serviceId * 7919 + Math.floor(today / day));
    const episodes = new Map<string, number>();
    let start = from;
    let eventId = 1;
    let previous: Template | undefined;
    // aired は、雛形ごとに、最後に流した放送日。
    const aired = new Map<Template, number>();
    while (start < to) {
      // その時刻に放送する雛形から選ぶ。局の傾向に合うものがなければ、時間帯の合うほかの雛形から選ぶ。
      // どれも選べなければ、埋め草になる。
      const hour = Math.floor(((start + jstOffset) % day) / (60 * minute));
      const onAir = (item: Template) =>
        (hour >= item.hours[0] && hour < item.hours[1]) ||
        (hour + 24 >= item.hours[0] && hour + 24 < item.hours[1]);
      const broadcastDay = Math.floor((start - from) / day);
      const usable = (item: Template) =>
        onAir(item) &&
        (item.channels === undefined || item.channels.includes(seed.serviceId)) &&
        (item.repeats === true || aired.get(item) !== broadcastDay);
      const candidates = seed.lineup.flatMap((genre) =>
        (templates[genre] ?? []).filter(usable).map((template) => ({ genre, template })),
      );
      const fallback = Object.entries(templates).flatMap(([genre, items]) =>
        items.filter(usable).map((template) => ({ genre: Number(genre), template })),
      );
      // 同じ番組が続かないよう、ほかに選べるなら、直前の雛形を除く。
      const choices =
        candidates.length > 0
          ? candidates
          : fallback.length > 0
            ? fallback
            : [{ genre: 2, template: filler }];
      const fresh = choices.filter((choice) => choice.template !== previous);
      const { genre: lv1, template } = pick(random, fresh.length > 0 ? fresh : choices);
      previous = template;
      aired.set(template, broadcastDay);
      const name = template.name(0);
      const episode = (episodes.get(name) ?? Math.floor(random() * 20)) + 1;
      episodes.set(name, episode);
      const end = start + pick(random, template.minutes) * minute;
      programs.push({
        __typename: 'Program',
        id: `Program:${seed.serviceId}:${eventId}`,
        service,
        eventId,
        startAt: new Date(start).toISOString(),
        endAt: new Date(end).toISOString(),
        endUndecided: false,
        isFree: true,
        name: template.name(episode),
        description: template.description,
        extended: [
          {
            heading: '番組内容',
            text: `${template.description}この番組はデモ用の架空の番組です。`,
          },
        ],
        genres: [{ lv1, lv2: template.lv2 }],
        audios: [{ componentType: 3, isMain: true, languages: ['jpn'], samplingRate: 48000 }],
        sharedFrom: null,
        start,
        end,
      });
      start = end;
      eventId += 1;
    }
  }

  return programs;
}

export type GenreInput = { lv1: number; lv2?: number | null };
export type KeywordInput = {
  text: string;
  regexp?: boolean | null;
  caseSensitive?: boolean | null;
  fields: string[];
};
export type TimeRangeInput = { weekdays: number[]; startMinute: number; durationMinutes: number };
export type ProgramConditionsInput = {
  keywords?: KeywordInput[] | null;
  keywordMatch?: string | null;
  excludeKeywords?: KeywordInput[] | null;
  channelTypes?: string[] | null;
  services?: string[] | null;
  excludeServices?: string[] | null;
  genres?: GenreInput[] | null;
  excludeGenres?: GenreInput[] | null;
  timeRanges?: TimeRangeInput[] | null;
  minDurationSeconds?: number | null;
  maxDurationSeconds?: number | null;
  freeOnly?: boolean | null;
};

function matchesKeyword(program: DemoProgram, keyword: KeywordInput): boolean {
  const texts = keyword.fields.map((field) =>
    field === 'NAME'
      ? program.name
      : field === 'DESCRIPTION'
        ? program.description
        : program.extended.map((item) => item.text).join('\n'),
  );
  const flags = keyword.caseSensitive === true ? 'u' : 'iu';
  if (keyword.regexp === true) {
    try {
      const pattern = new RegExp(keyword.text, flags);

      return texts.some((text) => pattern.test(text));
    } catch {
      return false;
    }
  }
  const needle = keyword.caseSensitive === true ? keyword.text : keyword.text.toLowerCase();

  return texts.some((text) =>
    (keyword.caseSensitive === true ? text : text.toLowerCase()).includes(needle),
  );
}

function matchesGenre(program: DemoProgram, genre: GenreInput): boolean {
  return program.genres.some(
    (item) => item.lv1 === genre.lv1 && (genre.lv2 == null || item.lv2 === genre.lv2),
  );
}

function matchesTimeRange(program: DemoProgram, range: TimeRangeInput): boolean {
  const local = new Date(program.start + jstOffset);
  const weekday = local.getUTCDay();
  const minuteOfDay = local.getUTCHours() * 60 + local.getUTCMinutes();
  // 日付をまたぐ範囲は、前の日の曜日の範囲として照合する。
  return [0, 1].some((back) => {
    const offset = minuteOfDay + back * 24 * 60 - range.startMinute;

    return (
      range.weekdays.includes((weekday + 7 - back) % 7) &&
      offset >= 0 &&
      offset < range.durationMinutes
    );
  });
}

// matchProgram は、検索やルールの条件 (ProgramConditionsInput) に、番組が当てはまるかを返す。
// tenban の照合 (設計書 3.5) を、デモで使う範囲だけ写したもの。マークの条件は扱わない。
export function matchProgram(program: DemoProgram, conditions: ProgramConditionsInput): boolean {
  const keywords = conditions.keywords ?? [];
  if (keywords.length > 0) {
    const results = keywords.map((keyword) => matchesKeyword(program, keyword));
    const matched =
      conditions.keywordMatch === 'ANY' ? results.some(Boolean) : results.every(Boolean);
    if (!matched) {
      return false;
    }
  }
  if ((conditions.excludeKeywords ?? []).some((keyword) => matchesKeyword(program, keyword))) {
    return false;
  }
  const channelTypeList = conditions.channelTypes ?? [];
  if (channelTypeList.length > 0 && !channelTypeList.includes(program.service.channelType.type)) {
    return false;
  }
  const serviceList = conditions.services ?? [];
  if (serviceList.length > 0 && !serviceList.includes(program.service.id)) {
    return false;
  }
  if ((conditions.excludeServices ?? []).includes(program.service.id)) {
    return false;
  }
  const genres = conditions.genres ?? [];
  if (genres.length > 0 && !genres.some((genre) => matchesGenre(program, genre))) {
    return false;
  }
  if ((conditions.excludeGenres ?? []).some((genre) => matchesGenre(program, genre))) {
    return false;
  }
  const ranges = conditions.timeRanges ?? [];
  if (ranges.length > 0 && !ranges.some((range) => matchesTimeRange(program, range))) {
    return false;
  }
  const seconds = (program.end - program.start) / 1000;
  if (conditions.minDurationSeconds != null && seconds < conditions.minDurationSeconds) {
    return false;
  }
  if (conditions.maxDurationSeconds != null && seconds > conditions.maxDurationSeconds) {
    return false;
  }
  if (conditions.freeOnly === true && !program.isFree) {
    return false;
  }

  return true;
}
