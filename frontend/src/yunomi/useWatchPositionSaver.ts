import { useEffect, useRef } from 'react';
import { graphql, useMutation } from 'react-relay';

import type { useWatchPositionSaverMutation } from './__generated__/useWatchPositionSaverMutation.graphql';

// saveInterval は、再生中に再生位置を保存する間隔 (ミリ秒)。
const saveInterval = 15_000;

const mutation = graphql`
  mutation useWatchPositionSaverMutation($id: ID!, $positionMs: Int64!) {
    saveWatchPosition(id: $id, positionMs: $positionMs) {
      id
      watchPositionMs
    }
  }
`;

// useWatchPositionSaver は、録画の再生位置を、再生中はしばらくごとに、止めたときと閉じたときに保存する (設計書 7.5)。
// position は、録画の始まりからの再生位置 (秒) を返す。
export function useWatchPositionSaver(
  recordingId: string,
  position: () => number,
  playing: boolean,
): void {
  const [commit] = useMutation<useWatchPositionSaverMutation>(mutation);
  const positionRef = useRef(position);
  positionRef.current = position;
  // saved は、最後に保存した位置 (ミリ秒)。変わっていなければ、保存しない。
  const saved = useRef<number | null>(null);

  const save = () => {
    const positionMs = Math.round(positionRef.current() * 1000);
    if (saved.current !== null && Math.abs(saved.current - positionMs) < 1000) {
      return;
    }
    saved.current = positionMs;
    // 保存できなくても再生は続ける。次の保存で取り戻す。
    commit({ variables: { id: recordingId, positionMs }, onError: () => {} });
  };
  const saveRef = useRef(save);
  saveRef.current = save;

  // 再生中は、しばらくごとに保存する。止めたら、その位置を保存する。
  // 開いた直後は、まだ再生を始めておらず、位置も決まっていないので、保存しない。
  const wasPlaying = useRef(false);
  useEffect(() => {
    if (!playing) {
      if (wasPlaying.current) {
        saveRef.current();
      }
      wasPlaying.current = false;

      return;
    }
    wasPlaying.current = true;
    const timer = setInterval(() => saveRef.current(), saveInterval);

    return () => clearInterval(timer);
  }, [playing]);

  // タブを閉じるときは、ページが消えても届くよう、keepalive の要求で保存する。画面を移るときは、Relay で保存する。
  useEffect(() => {
    const onPageHide = () => {
      // 一度も再生していなければ、保存するものがない。
      if (!wasPlaying.current && saved.current === null) {
        return;
      }
      const positionMs = Math.round(positionRef.current() * 1000);
      if (saved.current !== null && Math.abs(saved.current - positionMs) < 1000) {
        return;
      }
      saved.current = positionMs;
      void fetch('/graphql', {
        method: 'POST',
        keepalive: true,
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          query:
            'mutation ($id: ID!, $positionMs: Int64!) { saveWatchPosition(id: $id, positionMs: $positionMs) { id } }',
          variables: { id: recordingId, positionMs },
        }),
      }).catch(() => {});
    };
    window.addEventListener('pagehide', onPageHide);

    return () => {
      window.removeEventListener('pagehide', onPageHide);
      if (wasPlaying.current || saved.current !== null) {
        saveRef.current();
      }
    };
  }, [recordingId]);
}
