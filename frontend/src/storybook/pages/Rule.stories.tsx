import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent, within } from 'storybook/test';

import type { DemoRoot } from '../../demo';
import { GraphQLRequestError } from '../../relay/environment';
import { asViewer, call, merge } from '../conditions';
import { failing, failingWith, holdAll, PageStory } from '../PageStory';

// ルールの編集 (/rules/$ruleId) の画面 (#2056)。新しいルールと既存のルール、条件の入力欄 (src/conditions) の出し分け、
// プレビューの状態、保存と削除の結果、ほかの端末での変更を、Story ごとに並べる。
const meta = {
  title: 'ページ/ルール/編集',
  component: PageStory,
  args: { url: '/rules/Rule:1' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

type Node = Record<string, unknown>;

// rejecting は、名前が operation の読み込みを、error で失敗させる。hold の中で投げると、Relay に失敗として届く。
const rejecting = (operation: string, error: () => unknown) => (op: { name: string }) => {
  if (op.name === operation) {
    throw error();
  }

  return false;
};

// patchRule は、ルール id (既定は「こたつ探偵」の Rule:1) を、patch で変えて返す。
function patchRule(patch: (rule: Node, root: DemoRoot) => Node, id = 'Rule:1') {
  return (root: DemoRoot) => ({
    node: (args: { id: string }) => {
      const found = call<Node | null>(root, 'node', args);

      return args.id === id && found !== null ? { ...found, ...patch(found, root) } : found;
    },
  });
}

// richConditions は、条件の入力欄のすべての欄に値を入れたルール。照合のしかたは「どれか」、正規表現のキーワード、
// 除外するキーワード、上流から消えたサービス (#1973)、選択肢にない種別、テーブルにない囲み文字、時間帯と長さを含む。
const richConditions = patchRule((rule, root) => {
  const conditions = rule['conditions'] as Node;
  const services = call<Node[]>(root, 'services');

  return {
    conditions: {
      ...conditions,
      keywordMatch: 'ANY',
      keywords: [
        ...(conditions['keywords'] as Node[]),
        {
          text: 'こたつ.*探偵',
          regexp: true,
          caseSensitive: true,
          fields: ['NAME', 'DESCRIPTION'],
        },
      ],
      excludeKeywords: [{ text: '再放送', regexp: false, caseSensitive: false, fields: ['NAME'] }],
      channelTypes: ['GR', 'CS'],
      services: [
        services[0],
        {
          __typename: 'Service',
          id: 'Service:9999',
          name: '閉局した局',
          logoUrl: null,
          serviceId: 9999,
          gone: true,
          channelType: { type: 'GR', displayName: '地上波' },
        },
      ],
      excludeServices: [services[1]],
      genres: [{ lv1: 7, lv2: 0 }],
      excludeGenres: [{ lv1: 2, lv2: null }],
      timeRanges: [{ weekdays: [1, 2, 3, 4, 5], startMinute: 23 * 60, durationMinutes: 180 }],
      minDurationSeconds: 600,
      maxDurationSeconds: 3600,
      freeOnly: true,
      marks: ['新', '謎'],
      excludeMarks: ['再'],
    },
    settings: {
      ...(rule['settings'] as Node),
      startMarginSeconds: 30,
      endMarginSeconds: 60,
      priority: 5,
      joinSegments: true,
      detectCm: false,
      generateCaptions: true,
      transcodePreset: '720p',
      cmDetector: 'LOGO',
    },
    validFrom: new Date(Date.now() - 7 * 24 * 3_600_000).toISOString(),
    validUntil: new Date(Date.now() + 90 * 24 * 3_600_000).toISOString(),
    bundleSameEpisodes: true,
    skipEarlierEpisodes: true,
    followEventRelay: false,
  };
});

// withHiddenType は、上流にサービスのない種別 (CS) を足す。ルールが選んでいるときだけ、選択肢に残す (#1973)。
function withHiddenType(root: DemoRoot) {
  return {
    channelTypes: () => [
      ...call<Node[]>(root, 'channelTypes'),
      { type: 'CS', displayName: 'CS', sortOrder: 9, hasServices: false },
    ],
  };
}

// withGroup は、グループを作り、Rule:1 を入れる。
function withGroup(root: DemoRoot) {
  const group = call<{ id: string }>(root, 'createRuleGroup', { name: 'アニメ' });
  call(root, 'setRulesGroup', { ids: ['Rule:1'], groupId: group.id });

  return {};
}

const body = within(document.body);

// clickDelete は、ルールを消すボタンを押す。キーワードの行を消すボタン (アイコンだけで、名前が同じ「削除」) は押さない。
async function clickDelete(canvas: { findAllByRole: typeof body.findAllByRole }) {
  const buttons = await canvas.findAllByRole('button', { name: '削除' });
  const button = buttons.find((candidate) => candidate.textContent?.trim() === '削除');
  if (button === undefined) {
    throw new Error('the story could not find the delete button');
  }
  await userEvent.click(button);
}

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// ルールが見つからないとき (消されたルールの URL を開いたなど)。
export const NotFound: Story = { args: { url: '/rules/Rule:999' } };

// サーバーが、ルールが見つからない (NOT_FOUND) と応えたとき。
export const NotFoundByServer: Story = {
  args: { overrides: { storagePoolNames: failingWith({ code: 'NOT_FOUND' }) } },
};

// ルールを読み込めなかったとき。
export const LoadError: Story = { args: { overrides: { storagePoolNames: failing() } } };

// ログインが切れていて、ルールを読めなかったとき。
export const SignedOut: Story = {
  args: {
    hold: rejecting(
      'RuleIdQuery',
      () => new GraphQLRequestError('unauthenticated', 401, 'UNAUTHENTICATED'),
    ),
  },
};

// 新しいルール。番組を選ぶ条件がないので、保存できず、プレビューも取らない。削除のボタンと作成者の欄は出さない。
export const New: Story = { args: { url: '/rules/new' } };

// 新しいルールで、絞り込みの条件 (無料放送のみ) だけを入れたとき。プレビューに「条件なし」と示す (#1501)。
export const NewOnlyFilters: Story = {
  args: { url: '/rules/new' },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('switch', { name: '無料放送のみ' }));
  },
};

// 新しいルールを作れなかったとき。
export const CreateError: Story = {
  args: { url: '/rules/new', overrides: { createRule: failing() } },
  play: async ({ canvas }) => {
    await userEvent.type(await canvas.findByRole('textbox', { name: 'タイトル' }), '新しいルール');
    // 新しいルールには、キーワードの行がない。含めるキーワードの欄 (最初の「キーワードを追加」) に行を足す。
    const [addKeyword] = canvas.getAllByRole('button', { name: 'キーワードを追加' });
    await userEvent.click(addKeyword!);
    const keyword = await canvas.findByRole('textbox', { name: '含めるキーワード' });
    await userEvent.type(keyword, 'こたつ');
    await userEvent.click(canvas.getByRole('button', { name: '保存' }));
  },
};

// すべての条件と録画の設定を入れたルール。グループにも入れる。
export const RichConditions: Story = {
  args: { overrides: merge(richConditions, withHiddenType, withGroup) },
};

// 有効期間の始まりが終わりより後のとき。保存できない。
export const ValidityInvalid: Story = {
  args: {
    overrides: patchRule(() => ({
      validFrom: new Date(Date.now() + 30 * 24 * 3_600_000).toISOString(),
      validUntil: new Date(Date.now() + 10 * 24 * 3_600_000).toISOString(),
    })),
  },
};

// 視聴者から見たとき。入力欄を変えられず、保存と削除のボタンを出さない。
export const AsViewer: Story = { args: { overrides: asViewer('VIEWER') } };

// 保存したとき。
export const Saved: Story = {
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: '保存' }));
  },
};

// 保存できなかったとき。
export const SaveError: Story = {
  args: { overrides: { updateRule: failing() } },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: '保存' }));
  },
};

// ルールを消す前の確認。
export const DeleteConfirm: Story = {
  play: async ({ canvas }) => {
    await clickDelete(canvas);
  },
};

// ルールを消せなかったとき。ダイアログを閉じ、保存できなかった通知を出す。
export const DeleteError: Story = {
  args: { overrides: { deleteRule: failing() } },
  play: async ({ canvas }) => {
    await clickDelete(canvas);
    const dialog = within(await body.findByRole('dialog'));
    await userEvent.click(dialog.getByRole('button', { name: '削除' }));
  },
};

// captured は、Outdated の Story で作ったデモの既定の応答。play の中から、ほかの端末での変更を起こすのに使う。
// StrictMode で環境が 2 度作られても届くよう、作ったものをすべて保持する。
const captured: DemoRoot[] = [];

// 入力を触った後に、ほかの端末でルールが変わったとき (#533)。入力を残し、読み直すボタンを出す。
export const Outdated: Story = {
  // 前に開いたときの応答を残さず、この Story で作ったものにだけ変更を起こす。
  beforeEach: () => {
    captured.length = 0;
  },
  args: {
    overrides: (root: DemoRoot) => {
      captured.push(root);

      return {};
    },
  },
  play: async ({ canvas }) => {
    await userEvent.type(await canvas.findByRole('textbox', { name: 'タイトル' }), '（編集中）');
    for (const root of captured) {
      call(root, 'updateRule', {
        id: 'Rule:1',
        input: {
          name: 'こたつ探偵 (ほかの端末で変更)',
          conditions: { keywords: [{ text: 'こたつ探偵', fields: ['NAME'] }] },
        },
      });
    }
    // ほかの端末での変更の通知を受けて、読み直すまで少しかかる。
    await canvas.findByRole('button', { name: '最新の内容を読み込む' }, { timeout: 10_000 });
  },
};

// プレビューを読み込んでいるとき。
export const PreviewLoading: Story = {
  args: { hold: (operation) => operation.name === 'RulePreviewQuery' },
};

// プレビューに当たる番組がないとき。
export const PreviewEmpty: Story = {
  args: {
    overrides: {
      previewRule: () => ({
        edges: [],
        pageInfo: {
          hasNextPage: false,
          hasPreviousPage: false,
          startCursor: null,
          endCursor: null,
        },
      }),
    },
  },
};

// プレビューの番組が、出す件数より多いとき。
export const PreviewMore: Story = {
  args: {
    overrides: (root: DemoRoot) => ({
      previewRule: (args: object) => {
        const result = call<{ edges: unknown[]; pageInfo: Node }>(root, 'previewRule', args);

        return { ...result, pageInfo: { ...result.pageInfo, hasNextPage: true } };
      },
    }),
  },
};

// プレビューを取れなかったとき。
export const PreviewError: Story = { args: { overrides: { previewRule: failing() } } };

// 照合を重くするキーワード (#904) があるとき。保存は止めず、警告にとどめる。
export const SlowKeywordWarning: Story = {
  args: {
    overrides: merge(richConditions, () => ({
      ruleWarnings: () => [
        { kind: 'UNFILTERABLE_REGEXP', exclude: false, index: 1 },
        { kind: 'UNFILTERABLE_REGEXP', exclude: true, index: 0 },
      ],
    })),
  },
};

// プレビューの番組を押して、番組の詳細を開いたとき。
export const PreviewProgramDetail: Story = {
  play: async ({ canvas }) => {
    const list = within(await canvas.findByTestId('rule-preview'));
    const [program] = list.getAllByRole('button');
    await userEvent.click(program!);
  },
};

// ファイル名のテンプレートの書き方を開いたとき。
export const FileNameTemplateHelp: Story = {
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: '使える書き方を見る' }));
  },
};

// ファイル名のテンプレートが誤っているとき。
export const FileNameTemplateInvalid: Story = {
  args: {
    overrides: { previewFileName: () => ({ fileName: null, error: 'unknown field: Foo' }) },
  },
};

// ファイル名の例を作れなかったとき。
export const FileNameTemplatePreviewError: Story = {
  args: { overrides: { previewFileName: failing() } },
};

// 新しいグループを作って入れようとしたが、ほかのグループと同じ名前だったとき。
export const NewGroupInvalid: Story = {
  args: { overrides: withGroup },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByLabelText('グループ'));
    await userEvent.click(await body.findByRole('option', { name: '新しいグループ…' }));
    await userEvent.type(
      await canvas.findByRole('textbox', { name: '新しいグループの名前' }),
      'アニメ{Enter}',
    );
  },
};
