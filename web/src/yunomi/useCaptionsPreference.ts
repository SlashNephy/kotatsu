import { useLocalStorage } from '@mantine/hooks';

// useCaptionsPreference は、字幕を出すかどうかを返す。端末ごとの好みなので、ブラウザに覚えさせる。
export function useCaptionsPreference(): [boolean, (captions: boolean) => void] {
  const [captions, setCaptions] = useLocalStorage<boolean>({
    key: 'kotatsu.player.captions',
    defaultValue: true,
    getInitialValueInEffect: false,
  });

  return [captions, setCaptions];
}
