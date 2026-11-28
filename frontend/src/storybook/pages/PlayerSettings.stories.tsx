import type { Meta, StoryObj } from '@storybook/react-vite';

import type { DemoRoot } from '../../demo';
import { all, call, removeProperty, replaceProperty, withStorage } from '../conditions';
import { holdAll, PageStory } from '../PageStory';

// プレイヤーの設定 (/settings/player) の画面 (#2056)。アカウントの設定 (字幕、実況のソース) と、端末ごとの設定 (拡大、補間、
// 擬似 HDR) と、端末の機能 (別ウィンドウ、画面の向きの固定、タッチ) による出し分けを並べる。
const meta = {
  title: 'ページ/設定/プレイヤー',
  component: PageStory,
  args: { url: '/settings/player' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// 字幕を出さない設定のとき (字幕の見た目の項目を変えられない)。
export const CaptionsOff: Story = {
  args: {
    overrides: (root: DemoRoot) => ({
      playerSettings: () => ({ ...call<object>(root, 'playerSettings'), captionsEnabled: false }),
    }),
  },
};

// 管理者が、実況のコメントのソースを 2 つとも有効にしているとき。
export const AllCommentSources: Story = {
  args: {
    overrides: (root: DemoRoot) => ({
      systemSettings: () => ({
        ...call<object>(root, 'systemSettings'),
        commentSources: ['NX_JIKKYO', 'NICOLOG'],
      }),
    }),
  },
};

// 管理者が、実況のコメントを無効にしているとき (ソースの項目を出さない)。
export const NoCommentSources: Story = {
  args: {
    overrides: (root: DemoRoot) => ({
      systemSettings: () => ({ ...call<object>(root, 'systemSettings'), commentSources: [] }),
    }),
  },
};

// この端末で、映像の拡大と、フレームの補間を入れているとき (強さや補間の速さを選べる)。
export const UpscalingAndInterpolation: Story = {
  beforeEach: () =>
    withStorage({
      'kotatsu.player.superResolution': 'true',
      'kotatsu.player.interpolation': 'true',
    }),
};

// 擬似 HDR を出せないブラウザのとき。
export const HDRUnsupported: Story = {
  beforeEach: () =>
    all(
      removeProperty(WebGL2RenderingContext.prototype, 'drawingBufferToneMapping'),
      removeProperty(WebGL2RenderingContext.prototype, 'drawingBufferStorage'),
    ),
};

// 擬似 HDR を出せるブラウザで、画面が HDR を出せないとき。
export const HDRNoDisplay: Story = {
  beforeEach: () =>
    all(
      replaceProperty(
        WebGL2RenderingContext.prototype,
        'drawingBufferToneMapping',
        () => undefined,
      ),
      replaceProperty(WebGL2RenderingContext.prototype, 'drawingBufferStorage', () => undefined),
      matchMediaFor(false),
    ),
};

// 擬似 HDR を出せるブラウザで、画面も HDR を出せるとき。
export const HDRDisplay: Story = {
  beforeEach: () =>
    all(
      replaceProperty(
        WebGL2RenderingContext.prototype,
        'drawingBufferToneMapping',
        () => undefined,
      ),
      replaceProperty(WebGL2RenderingContext.prototype, 'drawingBufferStorage', () => undefined),
      matchMediaFor(true),
      withStorage({ 'kotatsu.player.hdrOutput': 'true' }),
    ),
};

// 別ウィンドウへの自動の切り替え、全画面での横向きの固定、タッチの操作を持つ端末 (タブレットなど) のとき。
export const TabletFeatures: Story = {
  beforeEach: () =>
    all(
      replaceProperty(window, 'documentPictureInPicture', { requestWindow: () => undefined }),
      replaceProperty(window.screen, 'orientation', {
        type: 'portrait-primary',
        lock: () => Promise.resolve(),
        unlock: () => undefined,
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
      }),
      replaceProperty(navigator, 'maxTouchPoints', 5),
    ),
};

// 別ウィンドウへの自動の切り替えと、画面の向きの固定を持たない端末のとき (その項目を出さない)。
export const NoDeviceFeatures: Story = {
  beforeEach: () =>
    all(
      removeProperty(window, 'documentPictureInPicture'),
      replaceProperty(window.screen, 'orientation', undefined),
      replaceProperty(navigator, 'maxTouchPoints', 0),
    ),
};

// matchMediaFor は、HDR の画面かを問う media query に、hdr を返す matchMedia に差し替える。ほかの問いは、元のまま答える。
function matchMediaFor(hdr: boolean): () => void {
  const original = window.matchMedia.bind(window);

  return replaceProperty(window, 'matchMedia', (query: string) =>
    query.includes('dynamic-range')
      ? {
          matches: hdr,
          media: query,
          onchange: null,
          addEventListener: () => undefined,
          removeEventListener: () => undefined,
          addListener: () => undefined,
          removeListener: () => undefined,
          dispatchEvent: () => false,
        }
      : original(query),
  );
}
