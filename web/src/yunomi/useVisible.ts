import { useEffect, useState, type RefObject } from 'react';

// useVisible は、要素が画面に入っていて、タブも隠れていないかを返す (設計書 7.2)。
// 画面の外に出たプレイヤーと、隠れたタブのプレイヤーは、再生を止めて、帯域と CPU を使わないようにする。
export function useVisible(ref: RefObject<Element | null>): boolean {
  const [intersecting, setIntersecting] = useState(true);
  const [tabVisible, setTabVisible] = useState(() => document.visibilityState !== 'hidden');

  useEffect(() => {
    const onVisibilityChange = () => setTabVisible(document.visibilityState !== 'hidden');
    document.addEventListener('visibilitychange', onVisibilityChange);

    return () => document.removeEventListener('visibilitychange', onVisibilityChange);
  }, []);

  useEffect(() => {
    const element = ref.current;
    if (element === null || typeof IntersectionObserver === 'undefined') {
      return;
    }
    const observer = new IntersectionObserver(
      (entries) => {
        const entry = entries.at(-1);
        if (entry !== undefined) {
          setIntersecting(entry.isIntersecting);
        }
      },
      // 少しでも見えていれば、見えているとみなす。
      { threshold: 0 },
    );
    observer.observe(element);

    return () => observer.disconnect();
  }, [ref]);

  return intersecting && tabVisible;
}
