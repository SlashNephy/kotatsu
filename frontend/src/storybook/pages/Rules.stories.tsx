import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent, within } from 'storybook/test';

import type { DemoRoot } from '../../demo';
import { GraphQLRequestError } from '../../relay/environment';
import { asViewer, call, merge, withStorage } from '../conditions';
import { failing, holdAll, PageStory } from '../PageStory';

// ルールの一覧 (/rules) の画面 (#2056)。ルールの状態 (無効、有効期間の外)、グループの見出しと操作、探す語と絞り込み、
// まとめての操作と、視聴ルールのタブを、Story ごとに並べる。
const meta = {
  title: 'ページ/ルール',
  component: PageStory,
  args: { url: '/rules' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

// signedOut は、operation の読み込みを、ログインしていないために拒まれたことにする。hold の中で投げると、Relay に失敗として届く。
const signedOut = (operation: string) => (op: { name: string }) => {
  if (op.name === operation) {
    throw new GraphQLRequestError('unauthenticated', 401, 'UNAUTHENTICATED');
  }

  return false;
};

const day = 24 * 3_600_000;

// withStates は、ルールの状態を散らす。「縁側の午後」を無効にし、有効期間の終わったルールを足す。
function withStates(root: DemoRoot) {
  // デモのルールの ID は、予約と通しで振るので、2 つ目のルールの ID は日によって変わる。名前で探す。
  const rule = call<{ id: string; name: string }[]>(root, 'rules').find(
    (candidate) => candidate.name === '縁側の午後',
  );
  if (rule === undefined) {
    throw new globalThis.Error('the demo has no rule 縁側の午後');
  }
  call(root, 'updateRule', {
    id: rule.id,
    input: {
      name: rule.name,
      enabled: false,
      conditions: { keywords: [{ text: '縁側の午後', fields: ['NAME'] }], channelTypes: ['GR'] },
    },
  });
  call(root, 'createRule', {
    input: {
      name: '夏の特番',
      memo: '去年の夏だけ録ったもの',
      conditions: { keywords: [{ text: '夏の特番', fields: ['NAME'] }] },
      validFrom: new Date(Date.now() - 120 * day).toISOString(),
      validUntil: new Date(Date.now() - 30 * day).toISOString(),
    },
  });

  return {};
}

// withGroups は、グループを 3 つ作り、「こたつ探偵」(Rule:1) を最初のグループに入れる。最後のグループは空にする。
function withGroups(root: DemoRoot) {
  const ids = ['アニメ', 'ドラマ', '特番'].map(
    (name) => call<{ id: string }>(root, 'createRuleGroup', { name }).id,
  );
  call(root, 'setRulesGroup', { ids: ['Rule:1'], groupId: ids[0] });

  return {};
}

const body = within(document.body);

// openGroupMenu は、グループ name の見出しの操作のメニューを開く。
async function openGroupMenu(canvasElement: HTMLElement, name: string) {
  await userEvent.click(
    await within(canvasElement).findByRole('button', { name: `${name}の操作` }),
  );
}

// startSelecting は、選ぶのを始め、表示中のルールをすべて選ぶ。
async function startSelecting(canvasElement: HTMLElement) {
  const canvas = within(canvasElement);
  await userEvent.click(await canvas.findByRole('button', { name: '選ぶ' }));
  await userEvent.click(canvas.getByRole('button', { name: /^表示中の\d+件を選ぶ$/ }));
}

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// ルールがまだ 1 つもないとき。
export const Empty: Story = { args: { overrides: { rules: () => [] } } };

export const Error: Story = { args: { overrides: { rules: failing() } } };

// ログインが切れていて、一覧を読めなかったとき。
export const SignedOut: Story = { args: { hold: signedOut('rulesListQuery') } };

// 無効のルールと、有効期間の外のルール (#464) があるとき。メモも出す。
export const States: Story = { args: { overrides: withStates } };

// 視聴者から見たとき。作成、選ぶ、グループのボタンと、有効のスイッチを出さない。
export const AsViewer: Story = {
  args: { overrides: merge(withStates, withGroups, asViewer('VIEWER')) },
};

// 有効と無効を切り替えられなかったとき。
export const ToggleError: Story = {
  args: { overrides: { setRuleEnabled: failing() } },
  play: async ({ canvas }) => {
    const [toggle] = await canvas.findAllByRole('switch', { name: '有効' });
    await userEvent.click(toggle!);
  },
};

// 探す語で絞ったとき。何件中の何件かを出す。
export const Searched: Story = {
  args: { url: '/rules?q=こたつ', overrides: withStates },
};

// 状態で絞り、名前の降順に並べたとき。
export const FilteredAndSorted: Story = {
  args: { url: '/rules?state=disabled&sort=name&order=desc', overrides: withStates },
};

// 予約のあるルールだけに絞ったとき。
export const WithReservationsOnly: Story = {
  args: { url: '/rules?reservations=some', overrides: withStates },
};

// 絞り込みに当たるルールがないとき。絞り込みを外すボタンを出す。
export const NoMatch: Story = { args: { url: '/rules?q=見つからない語' } };

// グループに分けたとき (#1493)。ルールのないグループの見出しと、「その他」の見出しも出す。
export const Groups: Story = { args: { overrides: withGroups } };

// 「その他」の見出しを折りたたんだとき。端末に保存した折りたたみを読む。
export const GroupCollapsed: Story = {
  args: { overrides: withGroups },
  beforeEach: () => withStorage({ 'kotatsu.rules.collapsedGroups': '["others"]' }),
};

// グループがあるときに探す語で絞ったとき。当たったルールを隠さないよう、見出しを折りたためなくする。
export const GroupsSearched: Story = {
  args: { url: '/rules?q=こたつ', overrides: withGroups },
  beforeEach: () => withStorage({ 'kotatsu.rules.collapsedGroups': '["others"]' }),
};

// 最初のグループのメニュー。上へは移せない。
export const GroupMenuFirst: Story = {
  args: { overrides: withGroups },
  play: async ({ canvasElement }) => openGroupMenu(canvasElement, 'アニメ'),
};

// 最後のグループのメニュー。下へは移せない。
export const GroupMenuLast: Story = {
  args: { overrides: withGroups },
  play: async ({ canvasElement }) => openGroupMenu(canvasElement, '特番'),
};

// グループの名前を、ほかのグループと同じにしようとしたとき。
export const GroupRenameInvalid: Story = {
  args: { overrides: withGroups },
  play: async ({ canvasElement }) => {
    await openGroupMenu(canvasElement, 'アニメ');
    await userEvent.click(await body.findByRole('menuitem', { name: '名前を変える' }));
    const name = await body.findByRole('textbox', { name: 'グループの名前' });
    await userEvent.clear(name);
    await userEvent.type(name, 'ドラマ{Enter}');
  },
};

// グループの名前を変えられなかったとき (入力の誤りでない失敗)。
export const GroupRenameError: Story = {
  args: { overrides: merge(withGroups, () => ({ renameRuleGroup: failing() })) },
  play: async ({ canvasElement }) => {
    await openGroupMenu(canvasElement, 'アニメ');
    await userEvent.click(await body.findByRole('menuitem', { name: '名前を変える' }));
    await userEvent.type(await body.findByRole('textbox', { name: 'グループの名前' }), '2{Enter}');
  },
};

// グループを消す前の確認。
export const GroupDeleteConfirm: Story = {
  args: { overrides: withGroups },
  play: async ({ canvasElement }) => {
    await openGroupMenu(canvasElement, 'アニメ');
    await userEvent.click(await body.findByRole('menuitem', { name: 'グループを削除' }));
  },
};

// グループを並べ替えられなかったとき。
export const GroupReorderError: Story = {
  args: { overrides: merge(withGroups, () => ({ reorderRuleGroups: failing() })) },
  play: async ({ canvasElement }) => {
    await openGroupMenu(canvasElement, 'アニメ');
    await userEvent.click(await body.findByRole('menuitem', { name: '下へ移す' }));
  },
};

// 空のグループを作るダイアログ。
export const CreateGroup: Story = {
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: 'グループを作る' }));
  },
};

// 作るグループの名前が、ほかのグループと同じとき。
export const CreateGroupInvalid: Story = {
  args: { overrides: withGroups },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: 'グループを作る' }));
    await userEvent.type(
      await body.findByRole('textbox', { name: 'グループの名前' }),
      'アニメ{Enter}',
    );
  },
};

// まとめて操作するルールを選んでいるとき。表示中のルールをすべて選んだので、選ぶボタンは「外す」になる。
export const Selecting: Story = {
  args: { overrides: withGroups },
  play: async ({ canvasElement }) => startSelecting(canvasElement),
};

// 選んだルールをまとめて無効にした結果。飛ばしたルールの件数も出す。
export const BulkResult: Story = {
  args: { overrides: { setRulesEnabled: () => ({ processed: 1, skipped: 1 }) } },
  play: async ({ canvasElement, canvas }) => {
    await startSelecting(canvasElement);
    await userEvent.click(canvas.getByRole('button', { name: /^選んだ\d+件を無効にする$/ }));
  },
};

// まとめての操作に失敗したとき。
export const BulkError: Story = {
  args: { overrides: { setRulesEnabled: failing() } },
  play: async ({ canvasElement, canvas }) => {
    await startSelecting(canvasElement);
    await userEvent.click(canvas.getByRole('button', { name: /^選んだ\d+件を有効にする$/ }));
  },
};

// 選んだルールをまとめて消す前の確認。
export const BulkDeleteConfirm: Story = {
  play: async ({ canvasElement, canvas }) => {
    await startSelecting(canvasElement);
    await userEvent.click(canvas.getByRole('button', { name: /^選んだ\d+件を削除$/ }));
  },
};

// 選んだルールをグループへ移すダイアログで、新しいグループの名前を入れているとき。名前を入れている間は移せない。
export const MoveToNewGroup: Story = {
  args: { overrides: withGroups },
  play: async ({ canvasElement, canvas }) => {
    await startSelecting(canvasElement);
    await userEvent.click(canvas.getByRole('button', { name: /^選んだ\d+件をグループへ移す$/ }));
    await userEvent.click(await body.findByRole('combobox', { name: '移す先' }));
    await userEvent.click(await body.findByRole('option', { name: '新しいグループ…' }));
  },
};

// 移す先の新しいグループを作れなかったとき (ほかのグループと同じ名前)。
export const MoveNewGroupInvalid: Story = {
  args: { overrides: withGroups },
  play: async ({ canvasElement, canvas }) => {
    await startSelecting(canvasElement);
    await userEvent.click(canvas.getByRole('button', { name: /^選んだ\d+件をグループへ移す$/ }));
    await userEvent.click(await body.findByRole('combobox', { name: '移す先' }));
    await userEvent.click(await body.findByRole('option', { name: '新しいグループ…' }));
    await userEvent.type(
      await body.findByRole('textbox', { name: '新しいグループの名前' }),
      'アニメ{Enter}',
    );
  },
};

// 視聴ルールのタブ。視聴ルールがないとき。
export const WatchEmpty: Story = { args: { url: '/rules?watch=true' } };

// withWatchRules は、視聴ルールを count 個作る。2 つ目は無効にする。
function withWatchRules(count: number) {
  return (root: DemoRoot) => {
    for (let index = 0; index < count; index++) {
      call(root, 'createWatchRule', {
        input: {
          name: index === 0 ? '縁側の午後' : `見たい番組 ${index + 1}`,
          enabled: index !== 1,
          conditions: {
            keywords: [
              { text: index === 0 ? '縁側の午後' : `見たい番組 ${index + 1}`, fields: ['NAME'] },
            ],
          },
        },
      });
    }

    return {};
  };
}

// 視聴ルールのタブ。有効と無効の視聴ルールが並ぶとき。
export const WatchRules: Story = {
  args: { url: '/rules?watch=true', overrides: withWatchRules(2) },
};

// 視聴ルールが上限まであるとき。作成のボタンを押せなくし、上限を通知する。
export const WatchRulesFull: Story = {
  args: { url: '/rules?watch=true', overrides: withWatchRules(50) },
};

// 視聴ルールの有効と無効を切り替えられなかったとき。
export const WatchToggleError: Story = {
  args: {
    url: '/rules?watch=true',
    overrides: merge(withWatchRules(2), () => ({ setWatchRuleEnabled: failing() })),
  },
  play: async ({ canvas }) => {
    const [toggle] = await canvas.findAllByRole('switch', { name: '有効' });
    await userEvent.click(toggle!);
  },
};
