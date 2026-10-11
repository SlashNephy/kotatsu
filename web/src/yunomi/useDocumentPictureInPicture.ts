import { useEffect, useRef, useState } from 'react';

// Document Picture-in-Picture の API。TypeScript の DOM の型には、まだない。
type DocumentPictureInPicture = {
  requestWindow: (options?: { width?: number; height?: number }) => Promise<Window>;
};

function api(): DocumentPictureInPicture | null {
  return (
    (window as { documentPictureInPicture?: DocumentPictureInPicture }).documentPictureInPicture ??
    null
  );
}

// useDocumentPictureInPicture は、視聴画面を別のウィンドウに出す (設計書 7.5)。
// container に、別のウィンドウの中の要素を返すので、そこへ React のポータルで描く。
// 別のウィンドウには、このページのスタイルシートを写して、同じ見た目にする。
export function useDocumentPictureInPicture(): {
  supported: boolean;
  container: HTMLElement | null;
  // className は、別のウィンドウの中で描く要素に付けるクラス。
  open: (width: number, height: number, className: string) => Promise<void>;
  close: () => void;
} {
  const [container, setContainer] = useState<HTMLElement | null>(null);
  const pipWindow = useRef<Window | null>(null);

  const close = () => {
    pipWindow.current?.close();
    pipWindow.current = null;
    setContainer(null);
  };

  const open = async (width: number, height: number, className: string) => {
    const pip = api();
    if (pip === null || pipWindow.current !== null) {
      return;
    }
    const win = await pip.requestWindow({ width, height });
    // スタイルシートを写す。nonce の付いた style も、写したものは nonce を引き継ぐので、CSP に拒まれない。
    for (const node of document.head.querySelectorAll('style, link[rel="stylesheet"]')) {
      win.document.head.append(node.cloneNode(true));
    }
    // 配色 (ライトとダーク) を引き継ぐ。
    for (const attribute of document.documentElement.attributes) {
      if (attribute.name.startsWith('data-')) {
        win.document.documentElement.setAttribute(attribute.name, attribute.value);
      }
    }
    win.document.body.style.margin = '0';
    win.document.body.style.background = 'black';
    const root = win.document.createElement('div');
    root.className = className;
    win.document.body.append(root);
    win.addEventListener('pagehide', () => {
      pipWindow.current = null;
      setContainer(null);
    });
    pipWindow.current = win;
    setContainer(root);
  };

  // 画面を離れるときは、別のウィンドウも閉じる。
  useEffect(() => () => pipWindow.current?.close(), []);

  return { supported: api() !== null, container, open, close };
}
