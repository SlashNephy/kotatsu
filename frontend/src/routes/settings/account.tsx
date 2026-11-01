import {
  Alert,
  Anchor,
  Button,
  Card,
  Checkbox,
  Group,
  NumberInput,
  Stack,
  Text,
  Title,
} from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Suspense, startTransition, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';

import {
  animeRecordCallbackError,
  destinationNames,
  type AnimeRecordDestinationName,
} from '../../animeRecords/destinations';
import { useDataChanges } from '../../changes/DataChanges';
import { ErrorBoundary, PageLoader, zonedFormat } from '../../futon';
import { isUnauthenticated } from '../../relay/environment';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';
import { settingsErrorMessage } from '../../settings/errors';
import { mutate } from '../../settings/mutate';
import type { accountBeginMutation } from './__generated__/accountBeginMutation.graphql';
import type { accountDisconnectMutation } from './__generated__/accountDisconnectMutation.graphql';
import type { accountQuery } from './__generated__/accountQuery.graphql';
import type {
  AnimeRecordSettingsInput,
  accountSettingsMutation,
} from './__generated__/accountSettingsMutation.graphql';

type AccountSearch = {
  // connected は、記録先と連携して戻ってきたときの記録先 (#927)。
  connected?: AnimeRecordDestinationName;
  // error は、記録先との連携に失敗して戻ってきたときの理由。
  error?: string;
};

export const Route = createFileRoute('/settings/account')({
  validateSearch: (search: Record<string, unknown>): AccountSearch => {
    const result: AccountSearch = {};
    if (search['connected'] === 'annict' || search['connected'] === 'anilist') {
      result.connected = search['connected'];
    }
    if (typeof search['error'] === 'string') {
      result.error = search['error'];
    }

    return result;
  },
  component: AccountPage,
});

const connectedAtFormat = zonedFormat({
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

// AccountPage は、外部のサービスのアカウントとの連携の画面 (#927、設計書 10.3)。見たアニメを、Annict と AniList に記録する。
// 管理者がしょぼいカレンダー、Annict、AniList との連携を無効にしているときは、設定を出さない。
function AccountPage() {
  const { t } = useTranslation();

  return (
    <Stack gap="md">
      <Anchor component={Link} to="/settings" size="sm">
        {t('settings.back')}
      </Anchor>
      <Title order={2}>{t('settings.pages.account.title')}</Title>
      <ErrorBoundary
        fallback={(error) =>
          isUnauthenticated(error) ? (
            <Alert color="amber">
              <Anchor component={Link} to="/login">
                {t('guide.signedOut')}
              </Anchor>
            </Alert>
          ) : (
            <Alert color="amber">{t('viewer.error')}</Alert>
          )
        }
      >
        <Suspense fallback={<PageLoader />}>
          <AnimeRecords />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

const query = graphql`
  query accountQuery {
    animeRecordSettings {
      enabled
      thresholdPercent
      watchingOnFirst
      watchingOnEpisode
      watchedOnLast
      destinations {
        destination
        available
        connection {
          accountName
          connectedAt
        }
      }
    }
  }
`;

function AnimeRecords() {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  // fetchKey を変えると、読み直す。連携を外した後や、設定を保存した後に使う。
  const [fetchKey, setFetchKey] = useState(0);
  // ほかの端末で連携や設定を変えたら、読み直す (#503)。
  const viewerChanges = useDataChanges(['VIEWER_SETTINGS']);
  const data = useLazyLoadQuery<accountQuery>(
    query,
    {},
    { fetchKey: `${fetchKey}:${viewerChanges}`, fetchPolicy: 'store-and-network' },
  );
  const settings = data.animeRecordSettings;
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const [running, setRunning] = useState(false);
  const refresh = () => startTransition(() => setFetchKey((key) => key + 1));
  // 戻ってきたときの結果は、一度出したら URL から外す。読み直したときに、もう一度出さないため。
  const clearSearch = () => void navigate({ search: {}, replace: true });
  const run = (operation: () => Promise<void>) => {
    setError(null);
    setSaved(false);
    setRunning(true);
    operation()
      .catch((caught: unknown) => setError(settingsErrorMessage(t, caught)))
      .finally(() => setRunning(false));
  };

  if (!settings.enabled) {
    return <Alert color="gray">{t('animeRecords.disabled')}</Alert>;
  }

  const connect = (destination: AnimeRecordDestinationName) =>
    run(async () => {
      const response = await mutate<accountBeginMutation>(
        environment,
        graphql`
          mutation accountBeginMutation($destination: AnimeRecordDestination!) {
            beginAnimeRecordConnection(destination: $destination)
          }
        `,
        { destination: destination === 'annict' ? 'ANNICT' : 'ANILIST' },
      );
      window.location.assign(response.beginAnimeRecordConnection);
    });
  const disconnect = (destination: AnimeRecordDestinationName) =>
    run(async () => {
      await mutate<accountDisconnectMutation>(
        environment,
        graphql`
          mutation accountDisconnectMutation($destination: AnimeRecordDestination!) {
            disconnectAnimeRecord(destination: $destination) {
              enabled
            }
          }
        `,
        { destination: destination === 'annict' ? 'ANNICT' : 'ANILIST' },
      );
      clearSearch();
      refresh();
    });
  const save = (input: AnimeRecordSettingsInput) =>
    run(async () => {
      await mutate<accountSettingsMutation>(
        environment,
        graphql`
          mutation accountSettingsMutation($input: AnimeRecordSettingsInput!) {
            updateAnimeRecordSettings(input: $input) {
              thresholdPercent
            }
          }
        `,
        { input },
      );
      setSaved(true);
      refresh();
    });
  const callbackError = search.error === undefined ? null : animeRecordCallbackError(search.error);

  return (
    <Stack gap="md">
      <Text size="sm" c="dimmed">
        {t('animeRecords.description')}
      </Text>
      {/* 連携すると、見たいリストの作品の放送を、予約の画面の候補に出す (#928)。 */}
      <Text size="sm" c="dimmed">
        {t('animeRecords.wishlistHint')}{' '}
        <Anchor
          size="sm"
          renderRoot={(props) => <Link {...props} to="/reservations" search={{ wishlist: true }} />}
        >
          {t('animeRecords.wishlistLink')}
        </Anchor>
      </Text>
      {error !== null && (
        <Alert color="red" role="alert" withCloseButton onClose={() => setError(null)}>
          {error}
        </Alert>
      )}
      {callbackError !== null && (
        <Alert color="red" role="alert" withCloseButton onClose={clearSearch}>
          {t(`animeRecords.callbackErrors.${callbackError}`)}
        </Alert>
      )}
      {search.connected !== undefined && (
        <Alert color="green" role="status" withCloseButton onClose={clearSearch}>
          {t('animeRecords.connected', { name: destinationNames[search.connected] })}
        </Alert>
      )}
      {settings.destinations.map((status) => {
        const name: AnimeRecordDestinationName =
          status.destination === 'ANNICT' ? 'annict' : 'anilist';

        return (
          <Card key={status.destination} withBorder>
            <Stack gap="xs">
              <Group justify="space-between" wrap="wrap">
                <Title order={3} size="h5">
                  {destinationNames[name]}
                </Title>
                {status.connection != null ? (
                  <Button
                    variant="default"
                    size="xs"
                    color="red"
                    disabled={running}
                    onClick={() => disconnect(name)}
                  >
                    {t('animeRecords.disconnect')}
                  </Button>
                ) : (
                  <Button
                    size="xs"
                    disabled={running || !status.available}
                    onClick={() => connect(name)}
                  >
                    {t('animeRecords.connect')}
                  </Button>
                )}
              </Group>
              {status.connection != null ? (
                <Text size="sm">
                  {t('animeRecords.connectedAs', {
                    account: status.connection.accountName,
                    date: connectedAtFormat.format(new Date(status.connection.connectedAt)),
                  })}
                </Text>
              ) : (
                <Text size="sm" c="dimmed">
                  {status.available
                    ? t('animeRecords.notConnected')
                    : t('animeRecords.notRegistered')}
                </Text>
              )}
              {/* AniList は、トークンを取り消す API を持たないので、AniList の画面からも外してもらう。 */}
              {name === 'anilist' && status.connection != null && (
                <Text size="xs" c="dimmed">
                  {t('animeRecords.aniListRevokeHint')}
                </Text>
              )}
            </Stack>
          </Card>
        );
      })}
      <RecordSettings
        key={`${settings.thresholdPercent}/${String(settings.watchingOnFirst)}/${String(settings.watchingOnEpisode)}/${String(settings.watchedOnLast)}`}
        initial={{
          thresholdPercent: settings.thresholdPercent,
          watchingOnFirst: settings.watchingOnFirst,
          watchingOnEpisode: settings.watchingOnEpisode,
          watchedOnLast: settings.watchedOnLast,
        }}
        busy={running}
        saved={saved}
        onSave={save}
      />
    </Stack>
  );
}

// RecordSettings は、見たと判定する割合と、作品の状態を変える条件 (既定ではどれも変えない) を変える。
function RecordSettings({
  initial,
  busy,
  saved,
  onSave,
}: {
  initial: AnimeRecordSettingsInput;
  busy: boolean;
  saved: boolean;
  onSave: (input: AnimeRecordSettingsInput) => void;
}) {
  const { t } = useTranslation();
  const [input, setInput] = useState(initial);
  const changed =
    input.thresholdPercent !== initial.thresholdPercent ||
    input.watchingOnFirst !== initial.watchingOnFirst ||
    input.watchingOnEpisode !== initial.watchingOnEpisode ||
    input.watchedOnLast !== initial.watchedOnLast;

  return (
    <Card withBorder>
      <Stack gap="sm">
        <Title order={3} size="h5">
          {t('animeRecords.settingsTitle')}
        </Title>
        <NumberInput
          label={t('animeRecords.threshold')}
          description={t('animeRecords.thresholdHelp')}
          min={1}
          max={100}
          suffix="%"
          maw={240}
          value={input.thresholdPercent}
          onChange={(value) =>
            setInput({ ...input, thresholdPercent: typeof value === 'number' ? value : 0 })
          }
        />
        <Text size="sm" fw={500}>
          {t('animeRecords.statusTitle')}
        </Text>
        <Checkbox
          label={t('animeRecords.watchingOnFirst')}
          checked={input.watchingOnFirst}
          onChange={(event) => setInput({ ...input, watchingOnFirst: event.currentTarget.checked })}
        />
        <Checkbox
          label={t('animeRecords.watchingOnEpisode')}
          checked={input.watchingOnEpisode}
          onChange={(event) =>
            setInput({ ...input, watchingOnEpisode: event.currentTarget.checked })
          }
        />
        <Checkbox
          label={t('animeRecords.watchedOnLast')}
          description={t('animeRecords.statusHelp')}
          checked={input.watchedOnLast}
          onChange={(event) => setInput({ ...input, watchedOnLast: event.currentTarget.checked })}
        />
        <Group justify="flex-end" gap="sm">
          {saved && !changed && (
            <Text size="sm" c="teal" role="status">
              {t('admin.saved')}
            </Text>
          )}
          <Button size="compact-sm" disabled={!changed || busy} onClick={() => onSave(input)}>
            {t('admin.save')}
          </Button>
        </Group>
      </Stack>
    </Card>
  );
}
