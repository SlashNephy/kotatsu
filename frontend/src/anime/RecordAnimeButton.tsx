import { Button, Text } from '@mantine/core';
import { Check } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useMutation } from 'react-relay';

import { graphQLErrorDetails } from '../relay/environment';
import type {
  RecordAnimeButtonMutation,
  RecordAnimeEpisodesInput,
} from './__generated__/RecordAnimeButtonMutation.graphql';

type Result = 'requested' | 'already' | 'notConnected' | 'failed';

// RecordAnimeButton は、見たアニメを、連携した Annict と AniList に手で記録するボタン (設計書 3.8、#1232)。
// 記録はジョブで送るので、頼んだことだけを示し、結果は、自動の記録と同じく通知で知らせる。視聴者も使える。
export function RecordAnimeButton({
  input,
  label,
}: {
  input: RecordAnimeEpisodesInput;
  label: string;
}) {
  const { t } = useTranslation();
  const [record, recording] = useMutation<RecordAnimeButtonMutation>(graphql`
    mutation RecordAnimeButtonMutation($input: RecordAnimeEpisodesInput!) {
      recordAnimeEpisodes(input: $input)
    }
  `);
  const [result, setResult] = useState<Result | null>(null);
  const onFailed = (error: unknown) => {
    const { code } = graphQLErrorDetails(error);
    setResult(code === 'CONFLICT' ? 'already' : code === 'NOT_FOUND' ? 'notConnected' : 'failed');
  };
  const onClick = () => {
    setResult(null);
    record({
      variables: { input },
      onCompleted: (_, errors) => {
        if (errors != null && errors.length > 0) {
          onFailed({ source: { errors } });

          return;
        }
        setResult('requested');
      },
      onError: onFailed,
    });
  };

  if (result === 'requested') {
    return (
      <Text size="xs" c="teal" role="status">
        <Check size={12} aria-hidden /> {t('anime.record.requested')}
      </Text>
    );
  }

  return (
    <>
      <Button size="compact-xs" variant="default" loading={recording} onClick={onClick}>
        {label}
      </Button>
      {result !== null && (
        <Text size="xs" c={result === 'already' ? 'dimmed' : 'red'} role="alert">
          {t(`anime.record.${result}`)}
        </Text>
      )}
    </>
  );
}
