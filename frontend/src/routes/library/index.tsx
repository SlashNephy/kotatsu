import {
  Alert,
  Anchor,
  Badge,
  Breadcrumbs,
  Card,
  Group,
  Stack,
  Table,
  Text,
  Title,
} from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Cloud, FileVideo, Folder, FolderOpen, HardDrive } from 'lucide-react';
import { Suspense, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql } from 'react-relay';

import {
  EmptyState,
  ErrorBoundary,
  PageLoader,
  formatBytes,
  useDocumentTitle,
  zonedFormat,
} from '../../futon';
import { graphQLErrorDetails } from '../../relay/environment';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';
import {
  breadcrumbs,
  isMyDrive,
  parentPath,
  parseLibrarySearch,
  type LibrarySearch,
} from '../../library/library';
import type { libraryListQuery } from './__generated__/libraryListQuery.graphql';
import type { libraryDirectoryQuery } from './__generated__/libraryDirectoryQuery.graphql';

export const Route = createFileRoute('/library/')({
  validateSearch: parseLibrarySearch,
  component: LibraryPage,
});

// formatModified は、更新日時を出す。日時のないもの (Google Drive のマイドライブの根元) は空にする。
function formatModified(modifiedAt: string): string {
  const date = new Date(modifiedAt);

  return date.getTime() > 0 ? modifiedFormat.format(date) : '';
}

const modifiedFormat = zonedFormat({
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

// LibraryPage は、録画として登録していない TS を、ライブラリのフォルダをたどって選ぶファイラー (設計書 5.8、#1734)。
// ライブラリを選んでいなければ、ライブラリの一覧を出す。ファイルを選ぶと、再生の画面へ移る。
function LibraryPage() {
  const { t } = useTranslation();
  const { library, path = '' } = Route.useSearch();
  useDocumentTitle([t('library.title')]);

  return (
    <Stack gap="md">
      <Title order={2}>{t('library.title')}</Title>
      <Text size="sm" c="dimmed">
        {t('library.description')}
      </Text>
      <ErrorBoundary
        resetKey={`${library ?? ''}:${path}`}
        fallback={(error) => (
          <Alert color="amber">
            {graphQLErrorDetails(error).code === 'NOT_FOUND'
              ? t('library.notFound')
              : t('library.error')}
          </Alert>
        )}
      >
        <Suspense fallback={<PageLoader />}>
          {library === undefined ? <LibraryList /> : <Directory library={library} path={path} />}
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

// FolderLink は、ファイラーで、ライブラリ library のディレクトリ path (根元は空) を開くリンク。library がなければ、ライブラリの一覧を開く。
function FolderLink({
  library,
  path,
  children,
}: {
  library: string | undefined;
  path: string;
  children: ReactNode;
}) {
  const search: LibrarySearch =
    library === undefined ? {} : { library, ...(path === '' ? {} : { path }) };

  return (
    <Anchor
      size="sm"
      style={{ wordBreak: 'break-all' }}
      renderRoot={(props) => <Link {...props} to="/library" search={search} />}
    >
      {children}
    </Anchor>
  );
}

// useLibraryKind は、ライブラリの種類から、表示名とバッジの色を返す関数を返す。
function useLibraryKind() {
  const { t } = useTranslation();

  return (kind: string) => {
    switch (kind) {
      case 'POOL':
        return { label: t('library.kind.pool'), color: 'blue' };
      case 'GOOGLE_DRIVE':
        return { label: t('library.kind.googleDrive'), color: 'teal' };
      default:
        return { label: t('library.kind.folder'), color: 'gray' };
    }
  };
}

// LibraryList は、ライブラリの一覧。
function LibraryList() {
  const { t } = useTranslation();
  const { libraries } = useLazyLoadQuery<libraryListQuery>(
    graphql`
      query libraryListQuery {
        libraries {
          name
          kind
        }
      }
    `,
    {},
  );
  const kindOf = useLibraryKind();
  if (libraries.length === 0) {
    return <EmptyState icon={HardDrive} title={t('library.empty')} hint={t('library.emptyHint')} />;
  }

  return (
    <Stack gap="xs">
      {libraries.map((library) => (
        <Card
          key={library.name}
          withBorder
          padding="sm"
          renderRoot={(props) => (
            <Link {...props} to="/library" search={{ library: library.name }} />
          )}
        >
          <Group gap="sm" wrap="nowrap">
            {library.kind === 'GOOGLE_DRIVE' ? (
              <Cloud size={18} aria-hidden />
            ) : (
              <HardDrive size={18} aria-hidden />
            )}
            <Text fw={500}>{library.name}</Text>
            <Badge variant="light" color={kindOf(library.kind).color}>
              {kindOf(library.kind).label}
            </Badge>
          </Group>
        </Card>
      ))}
    </Stack>
  );
}

// Directory は、ライブラリ library のディレクトリ path の中身。ディレクトリと、再生できるファイルだけを出す。
function Directory({ library, path }: { library: string; path: string }) {
  const { t } = useTranslation();
  const { libraryDirectory: directory } = useLazyLoadQuery<libraryDirectoryQuery>(
    graphql`
      query libraryDirectoryQuery($library: String!, $path: String!) {
        libraryDirectory(library: $library, path: $path) {
          library {
            kind
          }
          path
          names
          entries {
            name
            path
            directory
            size
            modifiedAt
          }
        }
      }
    `,
    { library, path },
    { fetchPolicy: 'store-and-network' },
  );
  const kind = directory.library.kind;
  // Google Drive のマイドライブの根元は、サーバーが英語の名前で返すので、訳して出す (#1902)。
  const crumbs = breadcrumbs(library, directory.path, directory.names).map((crumb) =>
    isMyDrive(kind, crumb.path) ? { ...crumb, name: t('library.myDrive') } : crumb,
  );

  return (
    <Stack gap="sm">
      <Breadcrumbs separatorMargin={4}>
        <FolderLink library={undefined} path="">
          {t('library.all')}
        </FolderLink>
        {crumbs.map((crumb, i) =>
          i === crumbs.length - 1 ? (
            <Text key={crumb.path} size="sm" fw={500}>
              {crumb.name}
            </Text>
          ) : (
            <FolderLink key={crumb.path} library={library} path={crumb.path}>
              {crumb.name}
            </FolderLink>
          ),
        )}
      </Breadcrumbs>
      {directory.entries.length === 0 ? (
        <EmptyState
          icon={FolderOpen}
          title={t('library.emptyDirectory')}
          hint={t('library.emptyDirectoryHint')}
        />
      ) : (
        <Table.ScrollContainer minWidth={0}>
          <Table highlightOnHover verticalSpacing="xs">
            <Table.Thead>
              <Table.Tr>
                <Table.Th>{t('library.name')}</Table.Th>
                <Table.Th ta="right" visibleFrom="xs">
                  {t('library.size')}
                </Table.Th>
                <Table.Th visibleFrom="sm">{t('library.modifiedAt')}</Table.Th>
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {directory.path !== '' && (
                <Table.Tr>
                  <Table.Td colSpan={3}>
                    <FolderLink library={library} path={parentPath(directory.path)}>
                      {t('library.up')}
                    </FolderLink>
                  </Table.Td>
                </Table.Tr>
              )}
              {directory.entries.map((entry) => (
                <Table.Tr key={entry.path}>
                  <Table.Td>
                    <Group gap={6} wrap="nowrap">
                      {entry.directory ? (
                        <Folder size={16} aria-hidden />
                      ) : (
                        <FileVideo size={16} aria-hidden />
                      )}
                      {entry.directory ? (
                        <FolderLink library={library} path={entry.path}>
                          {isMyDrive(kind, entry.path) ? t('library.myDrive') : entry.name}
                        </FolderLink>
                      ) : (
                        <Anchor
                          size="sm"
                          style={{ wordBreak: 'break-all' }}
                          renderRoot={(props) => (
                            <Link
                              {...props}
                              to="/library/play"
                              search={{ library, path: entry.path }}
                            />
                          )}
                        >
                          {entry.name}
                        </Anchor>
                      )}
                    </Group>
                    {/* 狭い画面では、大きさと日時を名前の下に出す。 */}
                    {!entry.directory && (
                      <Text size="xs" c="dimmed" hiddenFrom="sm">
                        {formatBytes(Number(entry.size))} ·{' '}
                        {modifiedFormat.format(new Date(entry.modifiedAt))}
                      </Text>
                    )}
                  </Table.Td>
                  <Table.Td ta="right" visibleFrom="xs" style={{ whiteSpace: 'nowrap' }}>
                    {entry.directory ? '' : formatBytes(Number(entry.size))}
                  </Table.Td>
                  <Table.Td visibleFrom="sm" style={{ whiteSpace: 'nowrap' }}>
                    {formatModified(entry.modifiedAt)}
                  </Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        </Table.ScrollContainer>
      )}
    </Stack>
  );
}
