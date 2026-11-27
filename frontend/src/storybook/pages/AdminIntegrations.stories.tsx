import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent } from 'storybook/test';

import type { DemoRoot } from '../../demo';
import { asViewer, call, merge, minutesAgo } from '../conditions';
import { failing, failingWith, holdAll, PageStory } from '../PageStory';

// 連携 (/admin/integrations) の画面 (#2056)。開いたときの本人確認、設定の保存 (再認証と失敗)、発行した外部プレイヤーの URL、
// Web Push の連絡先、アニメとドラマの連携、記録先のアプリ、Google Drive の接続とフォルダの選択、Webhook を並べる。
const meta = {
  title: 'ページ/管理/連携',
  component: PageStory,
  args: { url: '/admin/integrations' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

// settingsWith は、全体の設定のうち patch の項目を変える。ほかの項目は、デモのものを使う。
const settingsWith = (patch: object) => (root: DemoRoot) => ({
  systemSettings: () => ({ ...call<object>(root, 'systemSettings'), ...patch }),
});

const withSettings =
  (patch: object, overrides: object = {}) =>
  (root: DemoRoot) => ({
    ...settingsWith(patch)(root),
    ...overrides,
  });

const origin = window.location.origin;

const issuedStreamUrls = () => [
  {
    id: 'IssuedStreamUrl:1',
    userDisplayName: '家族',
    targetName: '架空のテレビ',
    live: true,
    createdAt: minutesAgo(30),
    expiresAt: minutesAgo(-330),
    lastUsedAt: minutesAgo(5),
  },
  {
    id: 'IssuedStreamUrl:2',
    userDisplayName: '管理者',
    targetName: '架空のドラマ 第3話',
    live: false,
    createdAt: minutesAgo(120),
    expiresAt: minutesAgo(-240),
    lastUsedAt: null,
  },
];

const webhooks = () => [
  {
    id: 'Webhook:1',
    name: '家族の Discord',
    url: 'https://discord.example.com/api/webhooks/0000/demo',
    format: 'DISCORD',
    kinds: ['RECORDING_FAILED', 'STORAGE_LOW'],
    enabled: true,
  },
  {
    id: 'Webhook:2',
    name: '止めている Slack',
    url: 'https://hooks.example.com/services/demo',
    format: 'SLACK',
    kinds: ['RECORDING_COMPLETED'],
    enabled: false,
  },
  {
    id: 'Webhook:3',
    name: '汎用の受け口',
    url: 'not a url',
    format: 'JSON',
    kinds: ['RESERVATION_CONFLICT'],
    enabled: true,
  },
];

const googleClient = {
  clientId: '000000000000-demo.apps.googleusercontent.com',
  configured: true,
  redirectUri: `${origin}/internal/auth/google/callback`,
  updatedAt: minutesAgo(600),
};

const folders = [
  { id: 'folder-anime', name: 'Anime', libraryName: 'anime' },
  { id: 'folder-movies', name: '映画', libraryName: 'movies' },
];

const connected = (patch: object = {}) => ({
  googleIntegration: () => ({
    ...googleClient,
    drive: { accountName: 'family@example.com', connectedAt: minutesAgo(1440), revokedAt: null },
    folders,
    ...patch,
  }),
});

// choices は、フォルダを選ぶ欄に並べる Drive のフォルダ。根元にマイドライブと共有ドライブ、マイドライブの下にフォルダを置く。
const choices = {
  googleDriveFolderChoices: ({ parentId }: { parentId?: string | null }) =>
    parentId == null
      ? [
          { id: 'root', name: 'My Drive' },
          { id: 'shared-1', name: '家族の共有ドライブ' },
        ]
      : parentId === 'root'
        ? [
            { id: 'folder-anime-2', name: 'Anime' },
            { id: 'folder-dramas', name: 'Dramas' },
          ]
        : [],
};

// configured は、どの連携も登録し、使っているとき。
const configured = withSettings(
  {
    mcpEnabled: true,
    networkActivityEnabled: true,
    hibpEnabled: true,
    webPushSubject: 'mailto:admin@example.com',
    animeLinkingEnabled: true,
    dramaLinkingEnabled: true,
    commentSources: ['NX_JIKKYO', 'NICOLOG'],
  },
  {
    issuedStreamUrls,
    webhooks,
    ...connected(),
    animeImportToken: () => ({ configured: true }),
    animeImports: () => [
      {
        source: 'SAYA_DEFINITIONS',
        checkedAt: minutesAgo(60),
        importedAt: minutesAgo(60),
        itemCount: 812,
        error: null,
      },
      {
        source: 'ARM',
        checkedAt: minutesAgo(30),
        importedAt: minutesAgo(1440),
        itemCount: 15320,
        error: 'GET https://arm.example.com/: 503 Service Unavailable',
      },
      {
        source: 'ANNICT_WORKS',
        checkedAt: minutesAgo(10),
        importedAt: null,
        itemCount: 0,
        error: null,
      },
    ],
    dramaLinking: () => ({ tokenConfigured: true, tokenHint: 'x9Qz', tokenUpdatedAt: null }),
    dramaTitleOverrides: () => [
      { titleKey: '架空のドラマ', tmdbSeriesId: 123456, updatedAt: minutesAgo(60) },
      { titleKey: '架空の情報番組', tmdbSeriesId: null, updatedAt: minutesAgo(120) },
    ],
    animeRecordApps: () => [
      {
        destination: 'ANNICT',
        clientId: 'demo-annict-client',
        configured: true,
        redirectUri: `${origin}/internal/auth/annict/callback`,
        updatedAt: minutesAgo(600),
      },
      {
        destination: 'ANILIST',
        clientId: null,
        configured: false,
        redirectUri: `${origin}/internal/auth/anilist/callback`,
        updatedAt: null,
      },
    ],
  },
);

// デモの既定。外部への問い合わせは使っておらず、Google のクライアントも Webhook もない。
export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// どの連携も登録し、使っているとき (取り込みの結果、TMDB の作品の上書き、Webhook、Google Drive のフォルダなど)。
export const Configured: Story = { args: { overrides: configured } };

// 開いたときに、しばらく再認証していないので、本人の確認を求めるとき。
export const ReauthRequired: Story = {
  args: { auth: { '/internal/auth/reauth': { recent: false } } },
};

// 設定を保存しようとして、再認証を求められたとき (本人確認のダイアログ)。
export const ReauthOnSave: Story = {
  args: {
    overrides: { updateSystemSettings: failingWith({ code: 'REAUTHENTICATION_REQUIRED' }) },
  },
  play: async ({ canvas }) => {
    await userEvent.click(
      await canvas.findByRole('switch', { name: /MCPのクライアントとの連携を有効にする/ }),
    );
  },
};

// 設定を保存できなかったとき。
export const SaveFailed: Story = {
  args: { overrides: { updateSystemSettings: failing() } },
  play: async ({ canvas }) => {
    await userEvent.click(
      await canvas.findByRole('switch', { name: /MCPのクライアントとの連携を有効にする/ }),
    );
  },
};

// 発行された外部プレイヤーの URL があるとき (ライブと録画、まだ使っていないもの)。
export const IssuedStreamUrls: Story = { args: { overrides: { issuedStreamUrls } } };

// 外部プレイヤーの URL の発行を切った後も、期限の過ぎていない URL が残っているとき (一覧を出す)。
export const IssuedStreamUrlsAfterDisabled: Story = {
  args: { overrides: withSettings({ externalPlayerUrlsEnabled: false }, { issuedStreamUrls }) },
};

// 外部プレイヤーの URL を発行させず、残っている URL もないとき (一覧を出さない)。
export const ExternalPlayerUrlsOff: Story = {
  args: { overrides: settingsWith({ externalPlayerUrlsEnabled: false }) },
};

// 外部プレイヤーの URL を取り消せなかったとき。
export const RevokeFailed: Story = {
  args: { overrides: { issuedStreamUrls, revokeStreamUrl: failing() } },
  play: async ({ canvas }) => {
    const [revoke] = await canvas.findAllByRole('button', { name: '取り消す' });
    if (revoke !== undefined) {
      await userEvent.click(revoke);
    }
  },
};

// Web Push の送り手の連絡先に、形の違うものを入れて欄を離れたとき。
export const WebPushSubjectInvalid: Story = {
  play: async ({ canvas }) => {
    await userEvent.type(await canvas.findByLabelText('送り手の連絡先'), 'admin@example.com');
    await userEvent.tab();
  },
};

// TMDB との連携を使うのに、トークンを登録していないとき。
export const DramaTokenMissing: Story = {
  args: { overrides: settingsWith({ dramaLinkingEnabled: true }) },
};

// TMDB のトークンを登録しているが、見分ける末尾の文字がないとき。
export const DramaTokenWithoutHint: Story = {
  args: {
    overrides: withSettings(
      { dramaLinkingEnabled: true },
      { dramaLinking: () => ({ tokenConfigured: true, tokenHint: null, tokenUpdatedAt: null }) },
    ),
  },
};

// 作品の上書きに、TMDB の作品として読めないものを入れたとき。
export const DramaSeriesInvalid: Story = {
  play: async ({ canvas }) => {
    await userEvent.type(await canvas.findByLabelText('番組名'), '架空のドラマ');
    await userEvent.type(await canvas.findByLabelText('TMDBの作品のIDかURL'), 'not-a-series');
  },
};

// 記録先のアプリのクライアント ID を書き換えたとき (登録のボタンを押せる)。
export const RecordAppEditing: Story = {
  play: async ({ canvas }) => {
    const [clientId] = await canvas.findAllByRole('textbox', { name: 'クライアントID' });
    if (clientId !== undefined) {
      await userEvent.type(clientId, 'demo-client');
    }
  },
};

// Google のクライアントを登録し、まだ Drive を接続していないとき。
export const GoogleNotConnected: Story = {
  args: {
    overrides: {
      googleIntegration: () => ({ ...googleClient, drive: null, folders: [] }),
    },
  },
};

// Drive を接続して戻ってきたとき。
export const GoogleConnected: Story = {
  args: { url: '/admin/integrations?connected=google-drive', overrides: connected() },
};

// Drive の接続に失敗して戻ってきたとき (Google で拒否した)。
export const GoogleCallbackError: Story = {
  args: {
    url: '/admin/integrations?error=google_denied',
    overrides: { googleIntegration: () => ({ ...googleClient, drive: null, folders: [] }) },
  },
};

// Drive の接続が切れたとき (Google で許可を取り消した)。フォルダは選べず、接続し直すよう促す。
export const GoogleRevoked: Story = {
  args: {
    overrides: connected({
      drive: {
        accountName: 'family@example.com',
        connectedAt: minutesAgo(10_000),
        revokedAt: minutesAgo(60),
      },
    }),
  },
};

// Drive を接続し、まだフォルダを選んでいないとき。
export const GoogleFoldersEmpty: Story = { args: { overrides: connected({ folders: [] }) } };

// 選んだフォルダのライブラリの名前を、ほかのライブラリと同じ名前に変えようとしたとき。
export const GoogleFolderRenameTaken: Story = {
  args: { overrides: connected() },
  play: async ({ canvas }) => {
    const [rename] = await canvas.findAllByRole('button', { name: '名前を変える' });
    if (rename !== undefined) {
      await userEvent.click(rename);
    }
    const input = await canvas.findByRole('textbox', { name: 'ライブラリの名前' });
    await userEvent.clear(input);
    await userEvent.type(input, 'movies');
  },
};

// フォルダを選ぶ欄を開いたとき (根元のマイドライブと共有ドライブ)。
export const GoogleFolderPicker: Story = {
  args: { overrides: { ...connected(), ...choices } },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: 'フォルダを追加' }));
  },
};

// フォルダを読んでいる途中。
export const GoogleFolderPickerLoading: Story = {
  args: {
    overrides: { ...connected(), ...choices },
    hold: ({ name }) => name === 'GoogleDriveFoldersChoicesQuery',
  },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: 'フォルダを追加' }));
  },
};

// フォルダを読めなかったとき。
export const GoogleFolderPickerFailed: Story = {
  args: { overrides: { ...connected(), googleDriveFolderChoices: failing() } },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: 'フォルダを追加' }));
  },
};

// フォルダを開き、ライブラリの名前の案がほかのライブラリと重なったとき (下にフォルダがあるとき)。
export const GoogleFolderChooseTaken: Story = {
  args: { overrides: { ...connected(), ...choices } },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: 'フォルダを追加' }));
    await userEvent.click(await canvas.findByRole('button', { name: 'マイドライブ' }));
    await userEvent.click(await canvas.findByRole('button', { name: 'Anime' }));
  },
};

// 下にフォルダのないフォルダを開き、使えない形のライブラリの名前を入れたとき。
export const GoogleFolderChooseInvalid: Story = {
  args: { overrides: { ...connected(), ...choices } },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: 'フォルダを追加' }));
    await userEvent.click(await canvas.findByRole('button', { name: '家族の共有ドライブ' }));
    const input = await canvas.findByLabelText('ライブラリの名前');
    await userEvent.clear(input);
    await userEvent.type(input, 'Family Drive');
  },
};

// Webhook を変えているとき。
export const WebhookEditing: Story = {
  args: { overrides: { webhooks } },
  play: async ({ canvas }) => {
    const [edit] = await canvas.findAllByRole('button', { name: '編集' });
    if (edit !== undefined) {
      await userEvent.click(edit);
    }
  },
};

// Webhook を削除するか確認しているとき。
export const WebhookConfirmDelete: Story = {
  args: { overrides: { webhooks } },
  play: async ({ canvas }) => {
    const [remove] = await canvas.findAllByRole('button', { name: '削除' });
    if (remove !== undefined) {
      await userEvent.click(remove);
    }
  },
};

// Webhook を作り、署名用の秘密キーを一度だけ出したとき。
export const WebhookCreated: Story = {
  args: {
    overrides: {
      createWebhook: ({ input }: { input: { name: string } }) => ({
        webhook: { id: 'Webhook:new', name: input.name },
        secret: 'kotatsu_demo_webhook_0123456789abcdef',
      }),
    },
  },
  play: async ({ canvas }) => {
    await userEvent.type(await canvas.findByRole('textbox', { name: /名前/ }), '録画の失敗');
    await userEvent.type(
      await canvas.findByRole('textbox', { name: /送信先URL/ }),
      'https://hooks.example.com/kotatsu',
    );
    await userEvent.click(await canvas.findByRole('checkbox', { name: '録画に失敗した' }));
    await userEvent.click(await canvas.findByRole('button', { name: 'Webhookを作成' }));
  },
};

// 管理者でないユーザーが開いたとき (サーバーが FORBIDDEN で拒否する)。全体の設定はナビゲーションも読むので、Webhook の一覧で拒否する。
export const Forbidden: Story = {
  args: {
    overrides: merge(asViewer('MEMBER'), () => ({ webhooks: failingWith({ code: 'FORBIDDEN' }) })),
  },
};
