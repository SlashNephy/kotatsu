import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent, within } from 'storybook/test';

import { asViewer, merge, replaceProperty } from '../conditions';
import { failing, failingWith, holdAll, PageStory } from '../PageStory';

// ログ (/admin/logs) の画面 (#2056)。出どころ (Kotatsu、チューナーサーバー) ごとの行と重さ、ロールを分けた構成の案内、
// 取れなかった出どころ、絞り込み、「もっと見る」、不具合の報告の確認の窓を並べる。
const meta = {
  title: 'ページ/管理/ログ',
  component: PageStory,
  args: { url: '/admin/logs' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

type Line = { time: number; level: string; inferred: boolean; text: string };

const now = Date.now();

const kotatsuLines: Line[] = [
  {
    time: now - 60_000,
    level: 'INFO',
    inferred: false,
    text: 'msg="recording started" name=架空のドラマ',
  },
  {
    time: now - 50_000,
    level: 'WARN',
    inferred: false,
    text: 'msg="drop detected" pid=0x100 count=3',
  },
  {
    time: now - 40_000,
    level: 'ERROR',
    inferred: false,
    text: 'msg="failed to deliver webhook" host=hooks.example.com',
  },
];

const tunerLines: Line[] = [
  { time: now - 55_000, level: 'INFO', inferred: false, text: 'tuner #0 opened' },
  { time: now - 45_000, level: 'WARN', inferred: true, text: 'warning: signal level is low' },
  { time: now - 35_000, level: 'ERROR', inferred: true, text: 'error: tuner #1 is busy' },
];

const source = (kind: string, lines: Line[], patch: object = {}) => ({
  kind,
  total: lines.length,
  failure: null,
  version: kind === 'KOTATSU' ? 'v2026.11.0' : '4.0.0',
  os: 'linux',
  arch: 'amd64',
  runtime: kind === 'KOTATSU' ? 'go1.26' : 'Mirakurun',
  lines,
  ...patch,
});

const allRoles = ['tenban', 'neko', 'mikan', 'nekonote', 'kyusu'];

const withReport = (sources: object[], roles: string[] = allRoles) => ({
  version: () => ({ version: 'v2026.11.0', revision: null, modified: false, latestRelease: null }),
  logReport: () => ({
    sources,
    redactHosts: ['tuner.example.com'],
    instanceId: '0f3c9a7e-1d2b-4c5a-8e6f-7a8b9c0d1e2f',
    roles,
  }),
  systemStatus: () => ({
    tuners: [
      { name: 'PX-DEMO', types: ['GR'] },
      { name: 'PX-DEMO', types: ['GR'] },
      { name: 'PX-DEMO-S', types: ['BS', 'CS'] },
    ],
  }),
});

const both = withReport([source('KOTATSU', kotatsuLines), source('TUNER_SERVER', tunerLines)]);

// many は、一度に描く行数 (100) を超える行。古い順に並べる。
const many: Line[] = Array.from({ length: 160 }, (_, index) => ({
  time: now - (160 - index) * 1000,
  level: index % 20 === 0 ? 'WARN' : 'INFO',
  inferred: false,
  text: `msg="segment written" index=${index} path=/recordings/demo/${index}.ts`,
}));

// デモの既定 (ログの出どころがなく、ロールも分からないので、ロールを分けた構成として案内する)。
export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// Kotatsu とチューナーサーバーの両方のログ。重さの印 (推定のものは枠だけ) と出どころの印を出す。
export const Sources: Story = { args: { overrides: both } };

// Kotatsu のログだけのとき (出どころの印を出さない)。
export const KotatsuOnly: Story = {
  args: { overrides: withReport([source('KOTATSU', kotatsuLines)]) },
};

// ロールを分けた構成で、ほかのロールのログを kubectl で見るよう案内するとき。
export const Split: Story = {
  args: {
    overrides: withReport(
      [source('KOTATSU', kotatsuLines), source('TUNER_SERVER', tunerLines)],
      ['tenban', 'neko'],
    ),
  },
};

// 出どころのログを取れなかったとき (取れなかった理由ごと)。
export const SourceFailed: Story = {
  args: {
    overrides: withReport([
      source('KOTATSU', kotatsuLines, { failure: 'UNSUPPORTED' }),
      source('TUNER_SERVER', [], { failure: 'TIMEOUT', total: 0 }),
    ]),
  },
};

// チューナーサーバーにつながらなかったか、エラーを返したとき。
export const SourceUnreachable: Story = {
  args: {
    overrides: withReport([
      source('KOTATSU', [], { failure: 'HTTP_ERROR', total: 0 }),
      source('TUNER_SERVER', [], { failure: 'UNREACHABLE', total: 0 }),
    ]),
  },
};

// 一度に描く行数を超えるとき (もっと見るのボタンを出す)。
export const ManyLines: Story = {
  args: { overrides: withReport([source('KOTATSU', many, { total: 5000 })]) },
};

// 警告とエラーだけに絞ったとき。
export const ProblemsOnly: Story = {
  args: { overrides: both },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('switch', { name: '警告とエラーだけ' }));
  },
};

// 出どころの 1 つを切り、検索に一致する行がないとき。
export const NoMatch: Story = {
  args: { overrides: both },
  play: async ({ canvas }) => {
    const [kotatsu] = await canvas.findAllByRole('checkbox');
    if (kotatsu !== undefined) {
      await userEvent.click(kotatsu);
    }
    await userEvent.type(await canvas.findByLabelText('ログを検索'), 'no such line');
  },
};

// 不具合の報告の確認の窓を開いたとき。
export const Issue: Story = {
  args: { overrides: both },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: 'Issue にする' }));
  },
};

// 報告をコピーしたとき。端末のクリップボードには書かず、書いたことにする。
export const IssueCopied: Story = {
  args: { overrides: both },
  beforeEach: () => replaceProperty(navigator, 'clipboard', { writeText: () => Promise.resolve() }),
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: 'Issue にする' }));
    const dialog = await within(document.body).findByRole('dialog');
    await userEvent.click(await within(dialog).findByRole('button', { name: 'コピー' }));
  },
};

// タイトルを書き足して URL の上限を超え、ログの古い行を削ったとき。
export const IssueTrimmed: Story = {
  args: { overrides: withReport([source('KOTATSU', many, { total: 5000 })]) },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: 'Issue にする' }));
    const dialog = await within(document.body).findByRole('dialog');
    await userEvent.click(await within(dialog).findByRole('textbox', { name: 'タイトル' }));
    await userEvent.paste(
      '録画が途中で止まり、そのあと同じチャンネルを選んでも映らなくなりました'.repeat(2),
    );
  },
};

// 管理者でないユーザーが開いたとき (サーバーが FORBIDDEN で断る)。
export const Forbidden: Story = {
  args: {
    overrides: merge(asViewer('MEMBER'), () => ({
      ...both,
      logReport: failingWith({ code: 'FORBIDDEN' }),
    })),
  },
};

// ログを読めなかったとき。
export const Failed: Story = { args: { overrides: { ...both, logReport: failing() } } };
