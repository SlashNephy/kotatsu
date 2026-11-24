import { type ReactNode, useEffect, useLayoutEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';

// PipHost は、children (プレイヤー) を、このページと別のウィンドウ (Document Picture-in-Picture) の間で、作り直さずに動かす (#1955)。
// プレイヤーは、いつも同じ要素 (host) の中にポータルで描き、host をこのページの表示先か、別のウィンドウの container に付け替える。
// 描き先を変えるとプレイヤーを作り直し、再生が一瞬止まるので、要素だけを動かす。動画の要素は、文書をまたいで動かしても、
// 同じ処理の中で付け直せば止まらない。別のウィンドウに出している間は、このページの表示先に placeholder を出す。
export function PipHost({
  container,
  placeholder,
  children,
}: {
  container: HTMLElement | null;
  placeholder: ReactNode;
  children: ReactNode;
}) {
  // host は、プレイヤーを描く要素。表示先の並びに影響しないよう、箱を作らない。
  const [host] = useState(() => {
    const element = document.createElement('div');
    element.style.display = 'contents';
    return element;
  });
  const slotRef = useRef<HTMLDivElement | null>(null);
  // 描いた後、見せる前に付け替える。
  useLayoutEffect(() => {
    const target = container ?? slotRef.current;
    if (target !== null && host.parentNode !== target) {
      target.append(host);
    }
    const win = container?.ownerDocument.defaultView;
    if (win === null || win === undefined) {
      return;
    }
    // 別のウィンドウが閉じるとき (閉じるボタンや、このページに戻す操作) は、閉じ終える前に、このページの表示先に戻す。
    // React が描き直すのを待つと、閉じた文書の中で動画が読み直され、再生位置が始めに戻る。
    const restore = () => {
      slotRef.current?.append(host);
    };
    win.addEventListener('pagehide', restore);
    return () => win.removeEventListener('pagehide', restore);
  }, [container, host]);
  useEffect(() => () => host.remove(), [host]);

  return (
    <>
      <div ref={slotRef} style={{ display: 'contents' }} />
      {container !== null && placeholder}
      {createPortal(children, host)}
    </>
  );
}
