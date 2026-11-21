import { type ReactNode, useEffect, useLayoutEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';

// PipHost は、children (プレイヤー) を、このページと別のウィンドウ (Document Picture-in-Picture) の間で、作り直さずに動かす (#1955)。
// プレイヤーは、いつも同じ要素 (host) の中にポータルで描き、host をこのページの置き場か、別のウィンドウの container に付け替える。
// 描き先を変えるとプレイヤーを作り直し、再生が一瞬止まるので、要素だけを動かす。動画の要素は、文書をまたいで動かしても、
// 同じ処理の中で付け直せば止まらない。別のウィンドウに出している間は、このページの置き場に placeholder を出す。
export function PipHost({
  container,
  placeholder,
  children,
}: {
  container: HTMLElement | null;
  placeholder: ReactNode;
  children: ReactNode;
}) {
  // host は、プレイヤーを描く要素。置き場の並びに影響しないよう、箱を作らない。
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
