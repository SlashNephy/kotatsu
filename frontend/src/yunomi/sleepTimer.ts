import { useEffect, useRef, useState } from 'react';

// スリープタイマー (#821、設計書 7.5)。選んだ時間が経つか、ライブ視聴で番組が終わったら、プレイヤーを止める。
// ブラウザから OS を眠らせることはできないので、プレイヤーを閉じて、チューナーと帯域を手放す。選択は覚えない。

// SleepChoice は、設定のメニューの選択肢。分の数か、番組の終わり (programEnd) か、切る (off)。
export type SleepChoice = 'off' | '15' | '30' | '60' | '90' | 'programEnd';

// sleepMinuteChoices は、時間で止める選択肢。
export const sleepMinuteChoices = ['15', '30', '60', '90'] as const;

// sleepDeadline は、choice を now に選んだときに止める時刻 (ミリ秒)。時刻で止めないなら null。
export function sleepDeadline(choice: SleepChoice, now: number): number | null {
  if (choice === 'off' || choice === 'programEnd') {
    return null;
  }

  return now + Number(choice) * 60_000;
}

// remainingMinutes は、止めるまでの残りを、分に切り上げて返す。
export function remainingMinutes(deadline: number, now: number): number {
  return Math.max(Math.ceil((deadline - now) / 60_000), 0);
}

// checkIntervalMs は、止める時刻を過ぎたかを確かめる間隔。
const checkIntervalMs = 1_000;

export type SleepTimer = {
  choice: SleepChoice;
  setChoice: (choice: SleepChoice) => void;
  // remaining は、時間で止めるときの残りの分。時間で止めないなら null。
  remaining: number | null;
  // fire は、今止める。番組の終わりで止めるときに、画面が呼ぶ。
  fire: () => void;
};

// useSleepTimer は、スリープタイマーを返す。時間になったら onStop を呼び、選択を切る。
export function useSleepTimer(onStop: () => void): SleepTimer {
  const [choice, setChoiceState] = useState<SleepChoice>('off');
  const [deadline, setDeadline] = useState<number | null>(null);
  const [now, setNow] = useState(() => Date.now());
  const stopRef = useRef(onStop);
  stopRef.current = onStop;

  const fire = () => {
    setChoiceState('off');
    setDeadline(null);
    stopRef.current();
  };
  const fireRef = useRef(fire);
  fireRef.current = fire;

  useEffect(() => {
    if (deadline === null) {
      return;
    }
    const timer = setInterval(() => {
      const current = Date.now();
      setNow(current);
      if (current >= deadline) {
        fireRef.current();
      }
    }, checkIntervalMs);

    return () => clearInterval(timer);
  }, [deadline]);

  return {
    choice,
    setChoice: (next) => {
      const current = Date.now();
      setChoiceState(next);
      setNow(current);
      setDeadline(sleepDeadline(next, current));
    },
    remaining: deadline === null ? null : remainingMinutes(deadline, now),
    fire,
  };
}
