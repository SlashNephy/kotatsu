import {
  Alert,
  Anchor,
  Button,
  Card,
  Group,
  Image,
  SimpleGrid,
  Stack,
  Text,
  TextInput,
  Title,
} from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Camera, Search } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { deleteCapture, listCaptures, matchesQuery, type CaptureEntry } from '../captures/history';
import { EmptyState, ProgramTitle, toHalfWidth, useDocumentTitle, zonedFormat } from '../futon';
import { useCaptureFileNameTemplate } from '../yunomi/captureFileNameTemplate';
import {
  captureFileName,
  captureTexts,
  copyBlob,
  saveBlob,
  withCaptureTexts,
} from '../yunomi/capture';

export const Route = createFileRoute('/captures')({
  component: CapturesPage,
});

const timeFormat = zonedFormat({
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
  weekday: 'short',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hourCycle: 'h23',
});

// CapturesPage は、この端末に残したキャプチャの履歴 (#457、設計書 7.5)。番組名とサービス名で探し、保存、コピー、削除ができる。
function CapturesPage() {
  const { t } = useTranslation();
  useDocumentTitle([t('captures.title')]);
  const [entries, setEntries] = useState<CaptureEntry[] | null>(null);
  const [failed, setFailed] = useState(false);
  const [query, setQuery] = useState('');
  useEffect(() => {
    listCaptures()
      .then(setEntries)
      .catch(() => setFailed(true));
  }, []);
  const shown = useMemo(
    () => (entries ?? []).filter((entry) => matchesQuery(entry, query)),
    [entries, query],
  );

  const remove = async (id: number) => {
    await deleteCapture(id);
    setEntries((list) => list?.filter((entry) => entry.id !== id) ?? null);
  };

  return (
    <Stack gap="sm">
      <Title order={2}>{t('captures.title')}</Title>
      <Text size="sm" c="dimmed">
        {t('captures.description')}
      </Text>
      {failed && <Alert color="amber">{t('captures.loadError')}</Alert>}
      {entries !== null && entries.length > 0 && (
        <TextInput
          leftSection={<Search size={16} aria-hidden />}
          aria-label={t('captures.search')}
          placeholder={t('captures.search')}
          value={query}
          onChange={(event) => setQuery(event.currentTarget.value)}
          maw={400}
        />
      )}
      {entries !== null && entries.length === 0 && (
        <EmptyState icon={Camera} title={t('captures.empty')} hint={t('captures.emptyHint')} />
      )}
      {entries !== null && entries.length > 0 && shown.length === 0 && (
        <Text c="dimmed">{t('captures.noMatch')}</Text>
      )}
      <SimpleGrid cols={{ base: 1, sm: 2, lg: 3 }}>
        {shown.map((entry) => (
          <CaptureCard key={entry.id} entry={entry} onDelete={() => void remove(entry.id)} />
        ))}
      </SimpleGrid>
    </Stack>
  );
}

function CaptureCard({ entry, onDelete }: { entry: CaptureEntry; onDelete: () => void }) {
  const { t } = useTranslation();
  const [template] = useCaptureFileNameTemplate();
  const [url, setUrl] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  useEffect(() => {
    const objectUrl = URL.createObjectURL(entry.image);
    setUrl(objectUrl);

    return () => URL.revokeObjectURL(objectUrl);
  }, [entry.image]);

  const save = async () => {
    const texts = captureTexts(
      entry.time,
      entry.serviceName,
      entry.programName,
      entry.caption ?? null,
    );
    saveBlob(
      await withCaptureTexts(entry.image, texts),
      captureFileName(entry.time, entry.serviceName, entry.programName, template),
    );
  };
  const copy = async () => {
    try {
      await copyBlob(entry.image);
      setMessage(t('player.copied'));
    } catch {
      setMessage(t('player.captureError'));
    }
  };

  return (
    <Card withBorder padding="sm">
      <Card.Section>
        {url !== null && (
          <Image src={url} alt={entry.programName ?? entry.serviceName} fit="contain" />
        )}
      </Card.Section>
      <Stack gap={4} mt="xs">
        {entry.programName !== null && <ProgramTitle name={entry.programName} />}
        <Text size="xs" c="dimmed">
          {toHalfWidth(entry.serviceName)} · {timeFormat.format(entry.time)}
        </Text>
        <Group gap="xs" mt={4}>
          <Button size="compact-sm" variant="default" onClick={() => void save()}>
            {t('captures.save')}
          </Button>
          <Button size="compact-sm" variant="default" onClick={() => void copy()}>
            {t('captures.copy')}
          </Button>
          {entry.recordingId !== null && (
            <RecordingLink recordingId={entry.recordingId} position={entry.position ?? null} />
          )}
          <Button size="compact-sm" variant="subtle" color="red" onClick={onDelete}>
            {t('captures.delete')}
          </Button>
        </Group>
        {message !== null && (
          <Text size="xs" c="dimmed" role="status">
            {message}
          </Text>
        )}
      </Stack>
    </Card>
  );
}

// RecordingLink は、キャプチャを撮った録画へのリンク。撮った位置が分かれば、その場面から再生する。分からなければ、録画の詳細を開く。
function RecordingLink({
  recordingId,
  position,
}: {
  recordingId: string;
  position: number | null;
}) {
  const { t } = useTranslation();

  return (
    <Anchor
      size="sm"
      renderRoot={(props) =>
        position === null ? (
          <Link {...props} to="/recordings/$recordingId" params={{ recordingId }} />
        ) : (
          <Link
            {...props}
            to="/recordings/$recordingId/play"
            params={{ recordingId }}
            search={{ t: Math.floor(position) }}
          />
        )
      }
    >
      {position === null ? t('captures.openRecording') : t('captures.playFrom')}
    </Anchor>
  );
}
