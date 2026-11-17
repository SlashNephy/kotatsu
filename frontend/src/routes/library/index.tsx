import {
  Alert,
  Anchor,
  Badge,
  Breadcrumbs,
  Button,
  Card,
  Group,
  Stack,
  Table,
  Text,
  TextInput,
  Title,
} from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Cloud, FileVideo, Folder, FolderOpen, HardDrive, Search } from 'lucide-react';
import { Suspense, useState, type FormEvent, type ReactNode } from 'react';
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
  parentPath,
  parseLibrarySearch,
  type LibrarySearch,
} from '../../library/library';
import type { libraryListQuery } from './__generated__/libraryListQuery.graphql';
import type { libraryDirectoryQuery } from './__generated__/libraryDirectoryQuery.graphql';
import type { librarySearchQuery } from './__generated__/librarySearchQuery.graphql';

export const Route = createFileRoute('/library/')({
  validateSearch: parseLibrarySearch,
  component: LibraryPage,
});

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
  const { library, path = '', q } = Route.useSearch();
  useDocumentTitle([t('library.title')]);

  return (
    <Stack gap="md">
      <Title order={2}>{t('library.title')}</Title>
      <Text size="sm" c="dimmed">
        {t('library.description')}
      </Text>
      <ErrorBoundary
        resetKey={`${library ?? ''}:${path}:${q ?? ''}`}
        fallback={(error) => (
          <Alert color="amber">
            {graphQLErrorDetails(error).code === 'NOT_FOUND'
              ? t('library.notFound')
              : graphQLErrorDetails(error).code === 'CONFLICT'
                ? t('library.busy')
                : t('library.error')}
          </Alert>
        )}
      >
        <Suspense fallback={<PageLoader />}>
          {library === undefined ? (
            <LibraryList />
          ) : (
            <Directory library={library} path={path} q={q} />
          )}
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
function Directory({ library, path, q }: { library: string; path: string; q: string | undefined }) {
  const { t } = useTranslation();
  const { libraryDirectory: directory } = useLazyLoadQuery<libraryDirectoryQuery>(
    graphql`
      query libraryDirectoryQuery($library: String!, $path: String!) {
        libraryDirectory(library: $library, path: $path) {
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
  const crumbs = breadcrumbs(library, directory.path, directory.names);

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
      <SearchBox
        key={`${library}:${directory.path}:${q ?? ''}`}
        library={library}
        path={directory.path}
        q={q}
      />
      {q !== undefined ? (
        <Suspense fallback={<PageLoader />}>
          <SearchResults library={library} path={directory.path} q={q} />
        </Suspense>
      ) : directory.entries.length === 0 ? (
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
                          {entry.name}
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
                    {modifiedFormat.format(new Date(entry.modifiedAt))}
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

// SearchBox は、今のディレクトリの下を、名前で探す欄 (#1902)。探す語は URL のクエリ (q) に置き、戻ると前の結果に戻れるようにする。
function SearchBox({ library, path, q }: { library: string; path: string; q: string | undefined }) {
  const { t } = useTranslation();
  const navigate = Route.useNavigate();
  const [text, setText] = useState(q ?? '');
  const go = (next: string | undefined) => {
    const search: LibrarySearch = {
      library,
      ...(path === '' ? {} : { path }),
      ...(next === undefined ? {} : { q: next }),
    };
    void navigate({ to: '/library', search });
  };

  return (
    <form
      role="search"
      onSubmit={(event: FormEvent) => {
        event.preventDefault();
        const trimmed = text.trim();
        go(trimmed === '' ? undefined : trimmed);
      }}
    >
      <Group gap="xs" wrap="nowrap">
        <TextInput
          size="sm"
          style={{ flex: 1 }}
          leftSection={<Search size={14} aria-hidden />}
          placeholder={t('library.search.placeholder')}
          aria-label={t('library.search.label')}
          value={text}
          maxLength={100}
          onChange={(event) => setText(event.currentTarget.value)}
        />
        <Button type="submit" size="sm" variant="default">
          {t('library.search.submit')}
        </Button>
        {q !== undefined && (
          <Button size="sm" variant="subtle" onClick={() => go(undefined)}>
            {t('library.search.clear')}
          </Button>
        )}
      </Group>
    </form>
  );
}

// SearchResults は、ディレクトリ path の下で、名前に q を含む再生できるファイル (#1902)。
function SearchResults({ library, path, q }: { library: string; path: string; q: string }) {
  const { t } = useTranslation();
  const { librarySearch: result } = useLazyLoadQuery<librarySearchQuery>(
    graphql`
      query librarySearchQuery($library: String!, $path: String!, $query: String!) {
        librarySearch(library: $library, path: $path, query: $query) {
          truncated
          hits {
            folderNames
            entry {
              name
              path
              size
              modifiedAt
            }
          }
        }
      }
    `,
    { library, path, query: q },
    { fetchPolicy: 'store-and-network' },
  );
  if (result.hits.length === 0) {
    return (
      <EmptyState
        icon={Search}
        title={t('library.search.empty', { query: q })}
        hint={t('library.search.emptyHint')}
      />
    );
  }

  return (
    <Stack gap="xs">
      <Text size="sm" c="dimmed">
        {t('library.search.count', { query: q, count: result.hits.length })}
      </Text>
      {result.truncated && (
        <Alert color="amber" variant="light">
          {t('library.search.truncated')}
        </Alert>
      )}
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
            {result.hits.map(({ entry, folderNames }) => (
              <Table.Tr key={entry.path}>
                <Table.Td>
                  <Group gap={6} wrap="nowrap">
                    <FileVideo size={16} aria-hidden />
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
                  </Group>
                  {/* 同じ名前のファイルを見分けられるよう、あるフォルダを添える。 */}
                  <Text size="xs" c="dimmed" style={{ wordBreak: 'break-all' }}>
                    {[library, ...folderNames].join(' / ')}
                  </Text>
                </Table.Td>
                <Table.Td ta="right" visibleFrom="xs" style={{ whiteSpace: 'nowrap' }}>
                  {formatBytes(Number(entry.size))}
                </Table.Td>
                <Table.Td visibleFrom="sm" style={{ whiteSpace: 'nowrap' }}>
                  {modifiedFormat.format(new Date(entry.modifiedAt))}
                </Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      </Table.ScrollContainer>
    </Stack>
  );
}
