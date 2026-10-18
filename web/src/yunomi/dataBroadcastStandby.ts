// 待機文書の見分け方 (#717)。
//
// テレビ朝日の /61/0000/stream.bml や、フジテレビの /40/0100/startup.bml は、見える文書だが、映像と音声のほかに何も描かず、
// d ボタンを受け取ってから中身のある文書へ移る。見えた時点で d ボタンを押すのをやめると、ここで止まる。
// 文書の名前では、中身のある文書と見分けられないので、描いているものと、d ボタンを待っているかで見分ける。

// mediaType は、映像と音声の object の arib-type。中の要素も含めて、描いているものに数えない。
const mediaType = /^(video|audio)\//i;

// isStandbyDocument は、文書が、d ボタンを待ち、映像と音声のほかに何も描いていない待機文書かどうかを返す。
// root は、web-bml が描いた文書の根 (bml 要素)。
export function isStandbyDocument(root: Element | null): boolean {
  if (root === null) {
    return false;
  }
  if (root.querySelector('beitem[type="DataButtonPressed"][subscribe="subscribe"]') === null) {
    return false;
  }
  const body = root.querySelector('body');
  if (body === null) {
    return false;
  }

  return !Array.from(body.querySelectorAll('*')).some(draws);
}

// draws は、要素が、映像と音声のほかに、何かを描いているかどうかを返す。
function draws(element: Element): boolean {
  const tag = element.tagName.toLowerCase();
  // arib-bg は、web-bml が文書の背景の色を塗るために置く要素で、どの文書にもある。
  if (tag === 'arib-bg' || element.closest('arib-bg') !== null) {
    return false;
  }
  const media = element.closest('object');
  if (media !== null && mediaType.test(media.getAttribute('arib-type') ?? '')) {
    return false;
  }
  const style = getComputedStyle(element);
  if (style.visibility === 'hidden' || style.display === 'none') {
    return false;
  }
  if (tag === 'img' || tag === 'object' || tag === 'input') {
    return true;
  }
  if (
    Array.from(element.childNodes).some(
      (node) => node.nodeType === Node.TEXT_NODE && node.textContent?.trim(),
    )
  ) {
    return true;
  }

  return (
    !transparent(style.backgroundColor) ||
    (style.backgroundImage !== '' && style.backgroundImage !== 'none')
  );
}

function transparent(color: string): boolean {
  return color === '' || color === 'transparent' || /^rgba\(.*,\s*0\)$/.test(color);
}

// renderedRoot は、BMLBrowser の今の文書の、描いた要素の根を返す。
// web-bml の BML の DOM は、スタイルを読めないので、元の要素 (公開していない node) を読む。web-bml を上げたら確かめ直す。
export function renderedRoot(
  content: { bmlDocument?: { documentElement?: unknown } } | undefined,
): Element | null {
  const node = (content?.bmlDocument?.documentElement as { node?: unknown } | undefined)?.node;

  return node instanceof Element ? node : null;
}
