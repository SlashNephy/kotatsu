import { useEffect, useRef } from 'react';
import { graphql, useMutation } from 'react-relay';

import type { useWatchPositionSaverMutation } from './__generated__/useWatchPositionSaverMutation.graphql';
import type { useWatchPositionSaverLibraryMutation } from './__generated__/useWatchPositionSaverLibraryMutation.graphql';

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

const libraryMutation = graphql`
  mutation useWatchPositionSaverLibraryMutation($id: ID!, $positionMs: Int64!) {
    saveLibraryWatchPosition(id: $id, positionMs: $positionMs) {
      id
      watchPositionMs
    }
  }
`;

// keepaliveQueries は、タブを閉じるときに keepalive の要求で送る、保存の mutation の文。Relay を通さずに送るので、文で持つ。
const keepaliveQueries = {
  recording:
    'mutation ($id: ID!, $positionMs: Int64!) { saveWatchPosition(id: $id, positionMs: $positionMs) { id } }',
  library:
    'mutation ($id: ID!, $positionMs: Int64!) { saveLibraryWatchPosition(id: $id, positionMs: $positionMs) { id } }',
} as const;

// useWatchPositionSaver は、録画の再生位置を、再生中はしばらくごとに、止めたときと閉じたときに保存する (設計書 7.5)。
// position は、録画の始まりからの再生位置 (秒) を返す。
export function useWatchPositionSaver(
  recordingId: string,
  position: () => number,
  playing: boolean,
): void {
  const [commit] = useMutation<useWatchPositionSaverMutation>(mutation);
  usePositionSaver('recording', recordingId, position, playing, (positionMs) =>
    commit({ variables: { id: recordingId, positionMs }, onError: () => {} }),
  );
}

// useLibraryPositionSaver は、ライブラリのファイル (録画として登録していない TS) の再生位置を、録画と同じく保存する (設計書 5.8)。
// fileId は LibraryFile.id、position は、ファイルの始まりからの再生位置 (秒) を返す。
export function useLibraryPositionSaver(
  fileId: string,
  position: () => number,
  playing: boolean,
): void {
  const [commit] = useMutation<useWatchPositionSaverLibraryMutation>(libraryMutation);
  usePositionSaver('library', fileId, position, playing, (positionMs) =>
    commit({ variables: { id: fileId, positionMs }, onError: () => {} }),
  );
}

// usePositionSaver は、再生位置を、再生中はしばらくごとに、止めたときと閉じたときに save で保存する。
// タブを閉じるときは、ページが消えても届くよう、kind の mutation を keepalive の要求で送る。
function usePositionSaver(
  kind: keyof typeof keepaliveQueries,
  id: string,
  position: () => number,
  playing: boolean,
  save: (positionMs: number) => void,
): void {
  const positionRef = useRef(position);
  positionRef.current = position;
  // saved は、最後に保存した位置 (ミリ秒)。変わっていなければ、保存しない。
  const saved = useRef<number | null>(null);

  const saveNow = () => {
    const positionMs = Math.round(positionRef.current() * 1000);
    if (saved.current !== null && Math.abs(saved.current - positionMs) < 1000) {
      return;
    }
    saved.current = positionMs;
    // 保存できなくても再生は続ける。次の保存で取り戻す。
    save(positionMs);
  };
  const saveRef = useRef(saveNow);
  saveRef.current = saveNow;

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
      void fetch('/internal/graphql', {
        method: 'POST',
        keepalive: true,
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ query: keepaliveQueries[kind], variables: { id, positionMs } }),
      }).catch(() => {});
    };
    window.addEventListener('pagehide', onPageHide);

    return () => {
      window.removeEventListener('pagehide', onPageHide);
      if (wasPlaying.current || saved.current !== null) {
        saveRef.current();
      }
    };
  }, [kind, id]);
}
