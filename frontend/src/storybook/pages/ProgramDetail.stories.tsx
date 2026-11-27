import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent, within } from 'storybook/test';

import type { DemoRoot } from '../../demo';
import type { Executor } from '../../demo/executor';
import { broadcastDay, dateOf } from '../../guide/layout';
import { asViewer, call } from '../conditions';
import { failing, PageStory } from '../PageStory';

// 番組表の番組の詳細 (/guide?program=...) のパネル (#2056)。録画予約の状態 (予約なし、番組予約、ルール予約、無効、
// 一部のみ、競合)、放送中と放送の終わった番組、ロールごとの操作、作品の欄、ダイアログと失敗の通知を、Story ごとに並べる。
// デモの番組の ID は、デモのデータの時刻から作るので、番組表の応答から条件に合う番組を選んで開く。
const meta = {
  title: 'ページ/番組表/番組の詳細',
  component: PageStory,
  args: { url: programPath((program, now) => program.start > now && program.reservation === null) },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

type Node = Record<string, unknown>;

type ListedProgram = {
  id: string;
  name: string;
  start: number;
  end: number;
  genre: number | undefined;
  type: string;
  reservation: { id: string; kind: string; enabled: boolean } | null;
};

type GuideResult = {
  data?: {
    guide?: {
      service: { channelType: { type: string } };
      programs: {
        id: string;
        name: string;
        startAt: string;
        endAt: string;
        genres: { lv1: number }[];
        reservation: ListedProgram['reservation'];
      }[];
    }[];
  };
};

// programPath は、デモの番組表 (昨日から 7 日後まで) から、pick に合う最初の番組を選び、その日と種別の番組表で、
// その番組の詳細を開くパスを返す。
function programPath(pick: (program: ListedProgram, now: number) => boolean) {
  return async (executor: Executor) => {
    const now = Date.now();
    const result = (await executor.execute(
      `query ($from: Time!, $to: Time!) {
        guide(from: $from, to: $to) {
          service { channelType { type } }
          programs { id name startAt endAt genres { lv1 } reservation { id kind enabled } }
        }
      }`,
      {
        from: new Date(now - 24 * 3_600_000).toISOString(),
        to: new Date(now + 7 * 24 * 3_600_000).toISOString(),
      },
    )) as GuideResult;
    const programs = (result.data?.guide ?? [])
      .flatMap((row) =>
        row.programs.map((program) => ({
          id: program.id,
          name: program.name,
          start: Date.parse(program.startAt),
          end: Date.parse(program.endAt),
          genre: program.genres[0]?.lv1,
          type: row.service.channelType.type,
          reservation: program.reservation,
        })),
      )
      .toSorted((a, b) => a.start - b.start);
    const found = programs.find((program) => pick(program, now));
    if (found === undefined) {
      throw new globalThis.Error('the demo has no program for the story');
    }
    const search = new URLSearchParams({
      date: dateOf(broadcastDay(found.start).from),
      type: found.type,
      program: found.id,
    });

    return `/guide?${search.toString()}`;
  };
}

// patchProgram は、node で取得する番組を、patch で変える。詳細のパネルは、開いた番組を node で取得する。
function patchProgram(patch: (program: Node, root: DemoRoot) => Node) {
  return (root: DemoRoot) => ({
    node: (args: { id: string }) => {
      const found = call<Node | null>(root, 'node', args);

      return found?.['__typename'] === 'Program' ? { ...found, ...patch(found, root) } : found;
    },
  });
}

// patchReservation は、開いた番組の予約を、patch で変える。
function patchReservation(patch: (reservation: Node, root: DemoRoot) => Node) {
  return patchProgram((program, root) => {
    const reservation = (program['reservation'] as () => Node | null)();

    return {
      reservation: reservation === null ? null : { ...reservation, ...patch(reservation, root) },
    };
  });
}

// peersOf は、チューナーを取り合っている相手の予約 (#1502) を、架空の番組名で作る。
function peersOf(root: DemoRoot, names: string[]) {
  const services = call<Node[]>(root, 'services');

  return names.map((name, index) => ({
    __typename: 'Reservation',
    id: `Reservation:peer-${index}`,
    name,
    service: services[index % services.length],
  }));
}

const upcoming = (program: ListedProgram, now: number) =>
  program.start > now && program.reservation === null;
const onAir = (program: ListedProgram, now: number) =>
  program.start <= now && program.end > now && program.reservation === null;
const ended = (program: ListedProgram, now: number) => program.end <= now;
const reservedBy =
  (kind: string, enabled = true) =>
  (program: ListedProgram, now: number) =>
    program.start > now &&
    program.reservation?.kind === kind &&
    program.reservation.enabled === enabled;

const body = within(document.body);

// openMore は、録画予約のボタンの横の ▾ のメニューを開く。
async function openMore() {
  await userEvent.click(await body.findByRole('button', { name: '録画予約のほかの操作' }));
}

// これから放送する、予約のない番組。ワンクリックの予約、▾ のメニュー、視聴予約のボタンを出す。
export const Upcoming: Story = {};

// 予約のない番組の ▾ のメニュー。設定して予約する項目と、ルールを作る項目を出す。
export const UpcomingMenu: Story = { play: openMore };

// 設定してから予約するダイアログ (#463)。
export const ConfigureBeforeReserve: Story = {
  play: async () => {
    await openMore();
    await userEvent.click(await body.findByRole('menuitem', { name: '設定して予約' }));
  },
};

// 予約できなかったとき。
export const ReserveError: Story = {
  args: { overrides: { createProgramReservation: failing() } },
  play: async () => {
    await userEvent.click(await body.findByRole('button', { name: '録画予約' }));
  },
};

// 視聴予約を付けられなかったとき。ベルのボタンに通知する。
export const WatchReserveError: Story = {
  args: { overrides: { setWatchReservation: failing() } },
  play: async () => {
    await userEvent.click(
      await body.findByRole('button', { name: '視聴予約（放送の少し前に通知します）' }),
    );
  },
};

// この番組からルールを作ったとき。番組名とサービスを条件に引き継いで、ルールの作成画面を開く (#1240)。
export const CreateRule: Story = {
  play: async () => {
    await openMore();
    await userEvent.click(await body.findByRole('menuitem', { name: 'この番組からルールを作成' }));
  },
};

// 番組予約のある番組。「録画予約済み」と出し、▾ のメニューに取り消しを出す。
export const ReservedProgram: Story = {
  args: { url: programPath(reservedBy('PROGRAM')) },
  play: openMore,
};

// 番組予約の録画の設定。録画予約済みのボタンを押して開く。
export const ReservedSettings: Story = {
  args: { url: programPath(reservedBy('PROGRAM')) },
  play: async () => {
    await userEvent.click(await body.findByRole('button', { name: '録画予約済み' }));
  },
};

// 番組予約を取り消す前の確認。番組予約は消す。
export const CancelProgramReservation: Story = {
  args: { url: programPath(reservedBy('PROGRAM')) },
  play: async () => {
    await openMore();
    await userEvent.click(await body.findByRole('menuitem', { name: '録画予約を取り消す' }));
  },
};

// 予約を取り消せなかったとき。
export const CancelError: Story = {
  args: { url: programPath(reservedBy('PROGRAM')), overrides: { deleteReservation: failing() } },
  play: async () => {
    await openMore();
    await userEvent.click(await body.findByRole('menuitem', { name: '録画予約を取り消す' }));
    await userEvent.click(await body.findByRole('button', { name: '取り消す' }));
  },
};

// ルール予約のある番組。「ルール録画予約済み」と出し、▾ のメニューに、もとになったルールを出す。
export const ReservedByRule: Story = {
  args: { url: programPath(reservedBy('RULE')) },
  play: openMore,
};

// ルール予約を取り消す前の確認。ルール予約は消さずに無効にすると説明する。
export const CancelRuleReservation: Story = {
  args: { url: programPath(reservedBy('RULE')) },
  play: async () => {
    await openMore();
    await userEvent.click(await body.findByRole('menuitem', { name: '録画予約を取り消す' }));
  },
};

// 複数のルールに一致したルール予約。録画の設定に使うルールに、そのことを添える。
export const ReservedByRules: Story = {
  args: {
    url: programPath(reservedBy('RULE')),
    overrides: patchReservation(() => ({
      rules: [
        { __typename: 'Rule', id: 'Rule:1', name: 'こたつ探偵' },
        { __typename: 'Rule', id: 'Rule:2', name: '深夜のアニメ' },
      ],
    })),
  },
  play: openMore,
};

// もとになったルールを消したルール予約。▾ のメニューに、ルールの見出しを出さない。
export const ReservedByDeletedRule: Story = {
  args: {
    url: programPath(reservedBy('RULE')),
    overrides: patchReservation(() => ({ rules: [] })),
  },
  play: openMore,
};

// 予約が無効になっている番組 (同じ回を録画済みと分かって Kotatsu が無効にしたなど)。予約していないものとして、予約のボタンを出す。
export const DisabledReservation: Story = { args: { url: programPath(reservedBy('RULE', false)) } };

// 一部のみ録画する予約。録れる予定の時刻と、チューナーを取り合っている相手を出す。
export const Partial: Story = {
  args: {
    url: programPath(reservedBy('RULE')),
    overrides: patchReservation((reservation, root) => ({
      assignment: {
        ...(reservation['assignment'] as Node),
        state: 'PARTIAL',
        plannedStartAt: new Date(
          Date.parse(String(reservation['startAt'])) + 5 * 60_000,
        ).toISOString(),
        plannedEndAt: reservation['endAt'],
        peers: peersOf(root, ['[新]縁側の午後']),
      },
    })),
  },
};

// 競合で録れない予約。チューナーを取り合っている相手を出す。名前のない相手は「名前なし」と出す。
export const Conflict: Story = {
  args: {
    url: programPath(reservedBy('RULE')),
    overrides: patchReservation((reservation, root) => ({
      assignment: {
        ...(reservation['assignment'] as Node),
        state: 'CONFLICT',
        tuner: null,
        plannedStartAt: null,
        plannedEndAt: null,
        peers: peersOf(root, ['[新]縁側の午後', '']),
      },
    })),
  },
};

// 放送中の番組。ライブ視聴のボタンを出し、視聴予約のボタンは出さない。
export const OnAir: Story = { args: { url: programPath(onAir) } };

// 放送の終わった番組。予約のボタンの代わりに、同じ番組名で探すボタンと、ルールを作るボタンを出す。
export const Ended: Story = { args: { url: programPath(ended) } };

// 放送の終わった番組に、録画とタイムシフトのバッファがあるとき (#1121)。録画とタイムシフトを開くボタンを出す。
export const EndedWithRecording: Story = {
  args: {
    url: programPath(ended),
    overrides: patchProgram((program) => ({
      recordings: [{ __typename: 'Recording', id: 'Recording:demo' }],
      service: {
        ...(program['service'] as Node),
        timeshiftPrograms: [{ eventId: program['eventId'], startAt: program['startAt'] }],
      },
    })),
  },
};

// 放送の終わった番組から、同じ番組名で探したとき。過去の放送も含めて、番組検索を開く。
export const SearchSameName: Story = {
  args: { url: programPath(ended) },
  play: async () => {
    await userEvent.click(await body.findByRole('button', { name: '同じ番組名で探す' }));
  },
};

// 視聴者から見たとき。録画予約とルールのボタンを出さず、視聴予約のボタンだけを出す。
export const AsViewer: Story = { args: { overrides: asViewer('VIEWER') } };

// 視聴者から、放送の終わった番組を見たとき。操作のボタンの行を出さない。
export const AsViewerEnded: Story = {
  args: { url: programPath(ended), overrides: asViewer('VIEWER') },
};

// 有料の番組で、終わりの時刻が決まっておらず、概要と詳しい説明がないとき。
export const PaidUndecidedNoDescription: Story = {
  args: {
    overrides: patchProgram(() => ({
      isFree: false,
      endUndecided: true,
      description: '',
      extended: [],
    })),
  },
};

// アニメの番組。番組名から読んだ話を、作品の欄に出す (設計書 3.8)。
export const Anime: Story = {
  args: {
    url: programPath(
      (program, now) =>
        upcoming(program, now) && program.genre === 7 && /#\d+|第\d+話/.test(program.name),
    ),
  },
};

// TMDB の作品に結びついたドラマの番組 (#1651)。作品の欄を出し、管理者には作品を直す操作も出す。
export const Drama: Story = {
  args: {
    url: programPath((program, now) => upcoming(program, now) && program.genre === 3),
    overrides: patchProgram(() => ({
      dramaEpisode: {
        series: {
          tmdbId: 1,
          name: '縁側の午後',
          originalName: 'Engawa no Gogo',
          url: 'https://example.com/tv/1',
          watchStatus: 'WATCHING',
        },
        seriesSource: 'SEARCH',
        seasonNumber: 1,
        episodeNumber: 3,
        episodeName: '冬の縁側',
        episodeSource: null,
        url: 'https://example.com/tv/1/season/1/episode/3',
        watched: false,
      },
    })),
  },
};

// ドラマの番組の、作品名でほかのサイトを探すメニュー。
export const SearchElsewhere: Story = {
  args: { url: programPath((program, now) => upcoming(program, now) && program.genre === 3) },
  play: async () => {
    await userEvent.click(await body.findByRole('button', { name: 'ほかのサイトで探す' }));
  },
};

// 番組の詳細を読み込んでいるとき。
export const Loading: Story = {
  args: { hold: (operation) => operation.name === 'ProgramDetailQuery' },
};

// 番組の詳細を読み込めなかったとき。
export const LoadError: Story = {
  args: {
    hold: (operation) => {
      if (operation.name === 'ProgramDetailQuery') {
        throw new globalThis.Error('the demo server failed');
      }

      return false;
    },
  },
};

// URL の番組が番組でないか、見つからないとき (消えた番組のリンクを開いたなど)。
export const NotFound: Story = {
  args: { url: () => Promise.resolve('/guide?program=Program:missing') },
};

// 視聴者に、予約がある番組を見せたとき。
export const AsViewerReserved: Story = {
  args: { url: programPath(reservedBy('RULE')), overrides: asViewer('VIEWER') },
};
