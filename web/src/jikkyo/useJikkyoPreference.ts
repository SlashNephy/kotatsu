import { useLocalStorage } from '@mantine/hooks';

// useJikkyoPreference は、実況のコメントを出すかどうかを返す。出さなければ、NX-Jikkyo に接続しない。
export function useJikkyoPreference(): [boolean, (enabled: boolean) => void] {
  const [enabled, setEnabled] = useLocalStorage<boolean>({
    key: 'kotatsu.player.jikkyo',
    defaultValue: true,
    getInitialValueInEffect: false,
  });

  return [enabled, setEnabled];
}
