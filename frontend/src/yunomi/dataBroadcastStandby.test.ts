import { describe, expect, test } from 'vitest';

import { isStandbyDocument, renderedRoot } from './dataBroadcastStandby';

// bml は、web-bml が描いた文書の根を、body の中身から作る。subscribe なら、d ボタンを待つ。
function bml(body: string, subscribe = true): Element {
  // HTML の構文解析では、要素の中の head と body を捨てるので、要素を作って組み立てる。
  const root = document.createElement('bml');
  const head = document.createElement('head');
  head.innerHTML = `<bevent><beitem type="DataButtonPressed" ${subscribe ? 'subscribe="subscribe"' : ''}></beitem></bevent>`;
  const content = document.createElement('body');
  content.innerHTML = body;
  root.append(head, content);
  document.body.append(root);

  return root;
}

const video =
  '<arib-bg style="background-color: rgb(0, 0, 0)"></arib-bg><div><object arib-type="video/X-arib-mpeg2"><div><video></video><canvas></canvas></div></object></div><object arib-type="audio/X-arib-mpeg2-aac"></object>';

describe('isStandbyDocument', () => {
  test.each([
    { name: '映像と音声だけで、d ボタンを待つ', root: () => bml(video), want: true },
    {
      name: '隠れた要素は数えない',
      root: () =>
        bml(
          `${video}<div style="visibility: hidden; background-color: rgb(255, 255, 0)"><p>debug</p></div>`,
        ),
      want: true,
    },
    { name: 'd ボタンを待たない', root: () => bml(video, false), want: false },
    { name: '文字を描く', root: () => bml(`${video}<p>ニュース</p>`), want: false },
    {
      name: '画像を描く',
      root: () => bml(`${video}<object arib-type="image/X-arib-png"></object>`),
      want: false,
    },
    {
      name: '色を塗る',
      root: () => bml(`${video}<div style="background-color: rgb(0, 128, 0)"></div>`),
      want: false,
    },
    { name: '文書がない', root: () => null, want: false },
  ])('$name', ({ root, want }) => {
    expect(isStandbyDocument(root())).toBe(want);
  });
});

describe('renderedRoot', () => {
  test('BML の DOM から、描いた要素を取り出す。取り出せなければ null', () => {
    const root = document.createElement('bml');
    expect(renderedRoot({ bmlDocument: { documentElement: { node: root } } })).toBe(root);
    expect(renderedRoot({ bmlDocument: { documentElement: {} } })).toBeNull();
    expect(renderedRoot(undefined)).toBeNull();
  });
});
