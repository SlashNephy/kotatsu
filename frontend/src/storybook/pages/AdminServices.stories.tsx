import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent } from 'storybook/test';

import type { DemoRoot } from '../../demo';
import { call, merge } from '../conditions';
import { failing, failingWith, holdAll, PageStory } from '../PageStory';

// サービスの管理 (/admin/services) の画面 (#2056)。放送波とサービスの表示、アニメの照合と実況のチャンネルの対応を並べる。
const meta = {
  title: 'ページ/管理/サービス',
  component: PageStory,
  args: { url: '/admin/services' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

type Service = Record<string, unknown> & { id: string; networkId: number; serviceId: number };
type AnimeChannel = Record<string, unknown> & { serviceId: string };

// marked は、最初のサービスを番組表に出さないマーク付きにし、2 つ目を上流から消えたものにして並び順の上書きを消す。
const marked = (root: DemoRoot) => ({
  services: () =>
    call<Service[]>(root, 'services').map((service, index) =>
      index === 0
        ? { ...service, hidden: true }
        : index === 1
          ? { ...service, gone: true, sortOrder: null }
          : service,
    ),
});

// linked は、アニメの連携を有効にし、実況のチャンネルの対応を 1 つ上書きした状態。
// アニメの照合は、1 つ目のサービスで上書きし、2 つ目は取り込んだ対応だけ、最後のサービスは対応がない (行を出さない)。
const linked = (root: DemoRoot) => {
  const [first] = call<Service[]>(root, 'services');

  return {
    systemSettings: () => ({
      ...call<object>(root, 'systemSettings'),
      animeLinkingEnabled: true,
      jikkyoChannels:
        first === undefined
          ? []
          : [{ networkId: first.networkId, serviceId: first.serviceId, channelId: 'jk101' }],
    }),
    animeChannels: () => {
      const channels = call<AnimeChannel[]>(root, 'animeChannels');

      return channels.slice(0, -1).map((channel, index) =>
        index === 0
          ? {
              ...channel,
              definitionName: '架空の定義',
              importedSyobocalChannelId: 1,
              importedAnnictChannelId: 101,
              syobocalChannelIdOverride: 2,
              annictChannelIdOverride: 202,
            }
          : index === 1
            ? {
                ...channel,
                definitionName: '架空の定義',
                importedSyobocalChannelId: 3,
                importedAnnictChannelId: 303,
              }
            : channel,
      );
    },
  };
};

// 連携が無効で、どのサービスも上書きしていないとき (デモの既定)。
export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// 管理者でないとき (サーバーが FORBIDDEN で拒んだ)。
export const Forbidden: Story = {
  args: { overrides: { channelTypes: failingWith({ code: 'FORBIDDEN' }) } },
};

// 設定を読めなかったとき。
export const Failed: Story = { args: { overrides: { channelTypes: failing() } } };

// 番組表に出さないサービスと、上流から消えたサービス (マークを付ける) があるとき。
export const HiddenAndGone: Story = { args: { overrides: marked } };

// アニメの連携が有効で、照合と実況のチャンネルの対応を上書きしているとき。対応のないサービスは、照合の一覧に出さない。
export const Linked: Story = { args: { overrides: merge(marked, linked) } };

// 実況のチャンネルの欄に、形の違う値を打って欄を離れたとき。
export const InvalidJikkyoChannel: Story = {
  play: async ({ canvas }) => {
    const [field] = await canvas.findAllByRole('textbox', { name: '実況のチャンネル（jk1など）' });
    if (field === undefined) {
      throw new Error('the story could not find field');
    }
    await userEvent.type(field, 'abc');
    await userEvent.tab();
  },
};

// 放送波の表示名を空にして欄を離れ、入力の誤りを出したとき。
export const ChannelTypeNameRequired: Story = {
  play: async ({ canvas }) => {
    const [field] = await canvas.findAllByRole('textbox', { name: '表示名' });
    if (field === undefined) {
      throw new Error('the story could not find field');
    }
    await userEvent.clear(field);
    await userEvent.tab();
  },
};

// サービスの設定を保存できなかったとき (失敗の知らせを出す)。
export const SaveFailed: Story = {
  args: { overrides: { updateService: failing() } },
  play: async ({ canvas }) => {
    const [hidden] = await canvas.findAllByRole('switch', { name: '出さない' });
    if (hidden === undefined) {
      throw new Error('the story could not find hidden');
    }
    await userEvent.click(hidden);
  },
};
