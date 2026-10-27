import { deinterlacerCanvas } from './deinterlacer';

// SavedStyle は、インラインのスタイルを変える前の要素と、その style 属性の中身。
export type SavedStyle = readonly [HTMLElement, string];

// fitMedia は、nodes とその中の要素を、移した先の枠いっぱいに広げ、変える前のインラインのスタイルを返す。
// データ放送は、映像の要素を web-bml の閉じた shadow root の中へ移し、そこには外の CSS が届かないので、大きさをインラインで整える。
// style 属性は CSP に止められるので、CSSOM で読み書きする。
export function fitMedia(nodes: readonly Node[]): SavedStyle[] {
  const elements = nodes.flatMap((node) =>
    node instanceof HTMLElement ? [node, ...node.querySelectorAll<HTMLElement>('*')] : [],
  );
  const saved = elements.map((element) => [element, element.style.cssText] as const);
  for (const element of elements) {
    // yadif の canvas は、yadif が video に合わせて置き直す。大きさを変えると、位置だけが元のままずれ、映像が二重に見える。
    if (element.dataset[deinterlacerCanvas] !== undefined) {
      continue;
    }
    element.style.display = 'block';
    element.style.width = '100%';
    element.style.height = '100%';
    if (element instanceof HTMLVideoElement || element instanceof HTMLCanvasElement) {
      element.style.objectFit = 'contain';
    }
  }

  return saved;
}
