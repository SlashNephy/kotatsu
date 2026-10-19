import {
  matchProgram,
  services,
  type DemoProgram,
  type DemoService,
  type ProgramConditionsInput,
} from './epg';

// デモの予約とルール (#857)。ルールに一致する番組からルール予約を作り、チューナーを割り当てる。
// 照合と割り当ては tenban (設計書 4.3、4.5) を簡単にしたもので、状態を変えるたびに作り直す。

export type SettingsInput = {
  poolId?: string | null;
  fileNameTemplate?: string | null;
  startMarginSeconds?: number | null;
  endMarginSeconds?: number | null;
  priority?: number | null;
  joinSegments?: boolean | null;
};

export type RuleInput = {
  name: string;
  memo?: string | null;
  enabled?: boolean | null;
  conditions: ProgramConditionsInput;
  settings?: SettingsInput | null;
  validFrom?: string | null;
  validUntil?: string | null;
  avoidDuplicates?: boolean | null;
  duplicateWindowDays?: number | null;
  followEventRelay?: boolean | null;
};

type Rule = { id: string; input: RuleInput; createdAt: number; updatedAt: number };

type Reservation = {
  id: string;
  kind: 'PROGRAM' | 'TIME' | 'RULE';
  program: DemoProgram | null;
  service: DemoService;
  start: number;
  end: number;
  name: string;
  enabled: boolean;
  followEventRelay: boolean;
  settings: SettingsInput;
  createdAt: number;
  updatedAt: number;
};

// tuners は、放送波ごとのチューナーの台数。番号は放送波をまたいで通しで振る。
const tuners: Record<string, number[]> = { GR: [0, 1], BS: [2, 3] };

export type Scheduler = ReturnType<typeof createScheduler>;

export function createScheduler(options: {
  programs: DemoProgram[];
  now: number;
  user: object;
  // changed は、予約やルールを変えたことを、画面に知らせる。
  changed: (channels: string[]) => void;
}) {
  const { programs, now, user, changed } = options;
  const programsById = new Map(programs.map((item) => [item.id, item]));
  const rules = new Map<string, Rule>();
  const reservations = new Map<string, Reservation>();
  let nextId = 1;
  const newId = (type: string) => `${type}:${nextId++}`;

  const ruleMatches = (rule: Rule, item: DemoProgram) => {
    const validFrom = rule.input.validFrom == null ? -Infinity : Date.parse(rule.input.validFrom);
    const validUntil = rule.input.validUntil == null ? Infinity : Date.parse(rule.input.validUntil);

    return (
      item.start > now &&
      !item.endUndecided &&
      item.start >= validFrom &&
      item.start < validUntil &&
      matchProgram(item, rule.input.conditions)
    );
  };

  const reservationOf = (programId: string) =>
    [...reservations.values()].find((item) => item.program?.id === programId);

  const matchingRules = (reservation: Reservation) =>
    reservation.program === null
      ? []
      : [...rules.values()].filter(
          (rule) => rule.input.enabled !== false && ruleMatches(rule, reservation.program!),
        );

  // syncRules は、有効なルールに一致する番組のうち、予約のないものにルール予約を作り、
  // どのルールにも一致しなくなったルール予約を消す。無効にしたルール予約は、一致する限り残す。
  const syncRules = () => {
    for (const reservation of reservations.values()) {
      if (reservation.kind === 'RULE' && matchingRules(reservation).length === 0) {
        reservations.delete(reservation.id);
      }
    }
    for (const rule of rules.values()) {
      if (rule.input.enabled === false) {
        continue;
      }
      for (const item of programs) {
        if (ruleMatches(rule, item) && reservationOf(item.id) === undefined) {
          const id = newId('Reservation');
          reservations.set(id, {
            id,
            kind: 'RULE',
            program: item,
            service: item.service,
            start: item.start,
            end: item.end,
            name: item.name,
            enabled: true,
            followEventRelay: rule.input.followEventRelay !== false,
            settings: {},
            createdAt: now,
            updatedAt: now,
          });
        }
      }
    }
  };

  // assignments は、有効な予約に、開始の早い順にチューナーを割り当てる。空いているチューナーがなければ競合にする。
  const assignments = () => {
    const result = new Map<string, { state: string; tuner: number | null }>();
    const busyUntil = new Map<number, number>();
    const active = [...reservations.values()]
      .filter((item) => item.enabled && item.end > now)
      .toSorted((a, b) => a.start - b.start);
    for (const reservation of active) {
      const candidates = tuners[reservation.service.channelType.type] ?? [];
      const tuner = candidates.find(
        (index) => (busyUntil.get(index) ?? -Infinity) <= reservation.start,
      );
      if (tuner === undefined) {
        result.set(reservation.id, { state: 'CONFLICT', tuner: null });
        continue;
      }
      busyUntil.set(tuner, reservation.end);
      result.set(reservation.id, { state: 'OK', tuner });
    }

    return result;
  };

  const settingsView = (settings: SettingsInput | null | undefined) => ({
    poolId: settings?.poolId ?? null,
    fileNameTemplate: settings?.fileNameTemplate ?? null,
    startMarginSeconds: settings?.startMarginSeconds ?? null,
    endMarginSeconds: settings?.endMarginSeconds ?? null,
    priority: settings?.priority ?? null,
    joinSegments: settings?.joinSegments ?? null,
  });

  const conditionsView = (conditions: ProgramConditionsInput) => ({
    keywords: (conditions.keywords ?? []).map((keyword) => ({
      text: keyword.text,
      regexp: keyword.regexp ?? false,
      caseSensitive: keyword.caseSensitive ?? false,
      fields: keyword.fields,
    })),
    keywordMatch: conditions.keywordMatch ?? 'ALL',
    excludeKeywords: (conditions.excludeKeywords ?? []).map((keyword) => ({
      text: keyword.text,
      regexp: keyword.regexp ?? false,
      caseSensitive: keyword.caseSensitive ?? false,
      fields: keyword.fields,
    })),
    channelTypes: conditions.channelTypes ?? [],
    services: services.filter((item) => (conditions.services ?? []).includes(item.id)),
    excludeServices: services.filter((item) =>
      (conditions.excludeServices ?? []).includes(item.id),
    ),
    genres: (conditions.genres ?? []).map((genre) => ({ lv1: genre.lv1, lv2: genre.lv2 ?? null })),
    excludeGenres: (conditions.excludeGenres ?? []).map((genre) => ({
      lv1: genre.lv1,
      lv2: genre.lv2 ?? null,
    })),
    timeRanges: conditions.timeRanges ?? [],
    minDurationSeconds: conditions.minDurationSeconds ?? null,
    maxDurationSeconds: conditions.maxDurationSeconds ?? null,
    freeOnly: conditions.freeOnly ?? false,
    marks: [],
    excludeMarks: [],
  });

  const ruleView = (rule: Rule) => ({
    __typename: 'Rule',
    id: rule.id,
    name: rule.input.name,
    memo: rule.input.memo ?? '',
    enabled: rule.input.enabled !== false,
    conditions: conditionsView(rule.input.conditions),
    settings: settingsView(rule.input.settings),
    validFrom: rule.input.validFrom ?? null,
    validUntil: rule.input.validUntil ?? null,
    avoidDuplicates: rule.input.avoidDuplicates ?? false,
    duplicateWindowDays: rule.input.duplicateWindowDays ?? null,
    followEventRelay: rule.input.followEventRelay !== false,
    createdAt: new Date(rule.createdAt).toISOString(),
    createdBy: user,
    updatedAt: new Date(rule.updatedAt).toISOString(),
    updatedBy: user,
    upcomingReservationCount: [...reservations.values()].filter(
      (item) =>
        item.kind === 'RULE' &&
        item.end > now &&
        item.program !== null &&
        ruleMatches(rule, item.program),
    ).length,
  });

  const reservationView = (reservation: Reservation, assigned = assignments()) => {
    const assignment = reservation.enabled
      ? (assigned.get(reservation.id) ?? { state: 'PENDING', tuner: null })
      : { state: 'PENDING', tuner: null };
    const startMargin = reservation.settings.startMarginSeconds ?? 0;
    const endMargin = reservation.settings.endMarginSeconds ?? 0;
    const planned = assignment.tuner !== null;

    return {
      __typename: 'Reservation',
      id: reservation.id,
      kind: reservation.kind,
      service: reservation.service,
      eventId: reservation.program?.eventId ?? null,
      startAt: new Date(reservation.start).toISOString(),
      endAt: new Date(reservation.end).toISOString(),
      name: reservation.name,
      description: reservation.program?.description ?? '',
      genres: reservation.program?.genres ?? [],
      enabled: reservation.enabled,
      followEventRelay: reservation.followEventRelay,
      settings: settingsView(reservation.settings),
      assignment: {
        state: assignment.state,
        tuner: assignment.tuner,
        plannedStartAt: planned
          ? new Date(reservation.start - startMargin * 1000).toISOString()
          : null,
        plannedEndAt: planned ? new Date(reservation.end + endMargin * 1000).toISOString() : null,
      },
      rules: matchingRules(reservation).map(ruleView),
      createdAt: new Date(reservation.createdAt).toISOString(),
      createdBy: user,
      updatedAt: new Date(reservation.updatedAt).toISOString(),
      updatedBy: user,
    };
  };

  const createRule = (input: RuleInput) => {
    const id = newId('Rule');
    const rule = { id, input, createdAt: now, updatedAt: now };
    rules.set(id, rule);
    syncRules();

    return rule;
  };

  const createProgramReservation = (
    programId: string,
    followEventRelay: boolean,
    settings: SettingsInput,
  ) => {
    const item = programsById.get(programId);
    if (item === undefined) {
      throw new Error('the program is not found');
    }
    const existing = reservationOf(programId);
    if (existing !== undefined) {
      return existing;
    }
    const id = newId('Reservation');
    const reservation: Reservation = {
      id,
      kind: 'PROGRAM',
      program: item,
      service: item.service,
      start: item.start,
      end: item.end,
      name: item.name,
      enabled: true,
      followEventRelay,
      settings,
      createdAt: now,
      updatedAt: now,
    };
    reservations.set(id, reservation);

    return reservation;
  };

  // 最初の状態。ルールを 2 つと、番組予約を 1 つ置く。
  createRule({
    name: 'こたつ探偵',
    memo: '深夜のアニメを毎話録る',
    conditions: { keywords: [{ text: 'こたつ探偵', fields: ['NAME'] }] },
    avoidDuplicates: true,
    duplicateWindowDays: 7,
  });
  createRule({
    name: '縁側の午後',
    conditions: {
      keywords: [{ text: '縁側の午後', fields: ['NAME'] }],
      channelTypes: ['GR'],
    },
  });
  const firstMovie = programs.find((item) => item.start > now && item.name.startsWith('映画'));
  if (firstMovie !== undefined) {
    createProgramReservation(firstMovie.id, true, {});
  }

  const reservationChanged = () => changed(['RESERVATIONS', 'ASSIGNMENTS']);
  const ruleChanged = () => changed(['RULES', 'RESERVATIONS', 'ASSIGNMENTS']);

  const ruleOrThrow = (id: string) => {
    const rule = rules.get(id);
    if (rule === undefined) {
      throw new Error('the rule is not found');
    }

    return rule;
  };
  const reservationOrThrow = (id: string) => {
    const reservation = reservations.get(id);
    if (reservation === undefined) {
      throw new Error('the reservation is not found');
    }

    return reservation;
  };

  // deleteOne は、予約を消す。ルール予約は、消すと次の照合でまた作られるので、消さずに無効にする。
  const deleteOne = (reservation: Reservation) => {
    if (reservation.kind === 'RULE') {
      reservation.enabled = false;
      reservation.updatedAt = now;

      return reservation;
    }
    reservations.delete(reservation.id);

    return null;
  };

  return {
    // 番組と node から引くもの
    reservationOfProgram: (programId: string) => {
      const reservation = reservationOf(programId);

      return reservation === undefined ? null : reservationView(reservation);
    },
    node: (id: string) => {
      const reservation = reservations.get(id);
      if (reservation !== undefined) {
        return reservationView(reservation);
      }
      const rule = rules.get(id);

      return rule === undefined ? undefined : ruleView(rule);
    },

    // Query
    rules: () => [...rules.values()].map(ruleView),
    reservations: (args: {
      from: string;
      to: string;
      states?: string[] | null;
      enabled?: boolean | null;
      rule?: string | null;
    }) => {
      const from = Date.parse(args.from);
      const to = Date.parse(args.to);
      const assigned = assignments();
      const rule = args.rule == null ? undefined : rules.get(args.rule);
      return [...reservations.values()]
        .filter((item) => item.start < to && item.end > from)
        .filter((item) => args.enabled == null || item.enabled === args.enabled)
        .filter(
          (item) =>
            args.rule == null ||
            (rule !== undefined && item.program !== null && ruleMatches(rule, item.program)),
        )
        .toSorted((a, b) => a.start - b.start)
        .map((item) => reservationView(item, assigned))
        .filter((item) => args.states == null || args.states.includes(item.assignment.state));
    },
    previewRule: (args: { input: RuleInput }) =>
      programs
        .filter((item) =>
          ruleMatches({ id: '', input: args.input, createdAt: now, updatedAt: now }, item),
        )
        .toSorted((a, b) => a.start - b.start),

    // Mutation
    createProgramReservation: (input: {
      program: string;
      followEventRelay?: boolean | null;
      settings?: SettingsInput | null;
    }) => {
      const reservation = createProgramReservation(
        input.program,
        input.followEventRelay !== false,
        input.settings ?? {},
      );
      reservationChanged();

      return reservationView(reservation);
    },
    createTimeReservation: (input: {
      service: string;
      startAt: string;
      endAt: string;
      name?: string | null;
      settings?: SettingsInput | null;
    }) => {
      const service = services.find((item) => item.id === input.service);
      if (service === undefined) {
        throw new Error('the service is not found');
      }
      const id = newId('Reservation');
      const reservation: Reservation = {
        id,
        kind: 'TIME',
        program: null,
        service,
        start: Date.parse(input.startAt),
        end: Date.parse(input.endAt),
        name: input.name ?? '',
        enabled: true,
        followEventRelay: false,
        settings: input.settings ?? {},
        createdAt: now,
        updatedAt: now,
      };
      reservations.set(id, reservation);
      reservationChanged();

      return reservationView(reservation);
    },
    updateReservation: (
      id: string,
      input: {
        enabled?: boolean | null;
        followEventRelay?: boolean | null;
        settings?: SettingsInput | null;
        startAt?: string | null;
        endAt?: string | null;
        name?: string | null;
      },
    ) => {
      const reservation = reservationOrThrow(id);
      if (input.enabled != null) {
        reservation.enabled = input.enabled;
      }
      if (input.followEventRelay != null) {
        reservation.followEventRelay = input.followEventRelay;
      }
      if (input.settings !== undefined) {
        reservation.settings = input.settings ?? {};
      }
      // 時刻と名前は、時刻予約でだけ変えられる。
      if (reservation.kind === 'TIME') {
        reservation.start = input.startAt == null ? reservation.start : Date.parse(input.startAt);
        reservation.end = input.endAt == null ? reservation.end : Date.parse(input.endAt);
        reservation.name = input.name ?? reservation.name;
      }
      reservation.updatedAt = now;
      reservationChanged();

      return reservationView(reservation);
    },
    deleteReservation: (id: string) => {
      const kept = deleteOne(reservationOrThrow(id));
      reservationChanged();

      return { id, reservation: kept === null ? null : reservationView(kept) };
    },
    deleteReservations: (ids: string[]) => {
      let processed = 0;
      for (const id of ids) {
        const reservation = reservations.get(id);
        if (reservation !== undefined) {
          deleteOne(reservation);
          processed += 1;
        }
      }
      reservationChanged();

      return { processed, skipped: ids.length - processed };
    },
    setReservationsEnabled: (ids: string[], enabled: boolean) => {
      let processed = 0;
      for (const id of ids) {
        const reservation = reservations.get(id);
        if (reservation !== undefined) {
          reservation.enabled = enabled;
          reservation.updatedAt = now;
          processed += 1;
        }
      }
      reservationChanged();

      return { processed, skipped: ids.length - processed };
    },
    createRule: (input: RuleInput) => {
      const rule = createRule(input);
      ruleChanged();

      return ruleView(rule);
    },
    updateRule: (id: string, input: RuleInput) => {
      const rule = ruleOrThrow(id);
      rule.input = input;
      rule.updatedAt = now;
      syncRules();
      ruleChanged();

      return ruleView(rule);
    },
    deleteRule: (id: string) => {
      ruleOrThrow(id);
      rules.delete(id);
      syncRules();
      ruleChanged();

      return id;
    },
    deleteRules: (ids: string[]) => {
      const processed = ids.filter((id) => rules.delete(id)).length;
      syncRules();
      ruleChanged();

      return { processed, skipped: ids.length - processed };
    },
    setRulesEnabled: (ids: string[], enabled: boolean) => {
      let processed = 0;
      for (const id of ids) {
        const rule = rules.get(id);
        if (rule !== undefined) {
          rule.input = { ...rule.input, enabled };
          rule.updatedAt = now;
          processed += 1;
        }
      }
      syncRules();
      ruleChanged();

      return { processed, skipped: ids.length - processed };
    },
    ruleView: (id: string) => ruleView(ruleOrThrow(id)),
  };
}
