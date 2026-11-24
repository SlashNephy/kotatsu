import { useEffect, useState } from 'react';
import { graphql, useRelayEnvironment } from 'react-relay';
import { fetchQuery } from 'relay-runtime';

import { useDataChanges } from '../changes/DataChanges';
import { setDRCSReplacements } from './drcs';
import type { useDRCSReplacementsQuery } from './__generated__/useDRCSReplacementsQuery.graphql';

const query = graphql`
  query useDRCSReplacementsQuery {
    drcsReplacements {
      md5
      text
    }
    viewer {
      role
    }
  }
`;

// useDRCSReplacements は、字幕の外字の置き換えのテーブルを読み、字幕のレンダラーが共有するテーブルに入れる (#825)。
// 字幕を描く画面で呼ぶ。テーブルは再生に欠かせないものではないので、読み終えるのを待たずに再生を始め、読めなくても失敗にしない。
// 管理者がテーブルを変えたら、読み直す。返す値は、外字を登録できる (管理者である) かどうか。
export function useDRCSReplacements(): { canRegister: boolean } {
  const environment = useRelayEnvironment();
  const adminChanges = useDataChanges(['ADMIN']);
  const [canRegister, setCanRegister] = useState(false);
  useEffect(() => {
    const subscription = fetchQuery<useDRCSReplacementsQuery>(environment, query, {}).subscribe({
      next: (data) => {
        setDRCSReplacements(data.drcsReplacements);
        setCanRegister(data.viewer.role === 'ADMIN');
      },
      error: () => {},
    });

    return () => subscription.unsubscribe();
  }, [environment, adminChanges]);

  return { canRegister };
}
