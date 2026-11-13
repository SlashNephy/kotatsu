import { Alert, Anchor, Card, Stack, Text, TextInput, Title } from '@mantine/core';
import { createFileRoute } from '@tanstack/react-router';
import { Suspense, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql } from 'react-relay';

import { LicenseList } from '../about/LicenseList';
import {
  bundledData,
  fetchBundledPackages,
  filterEntries,
  imagePrograms,
  repositoryURL,
  sourceURL,
  type SoftwareEntry,
} from '../about/licenses';
import { ErrorBoundary, PageLoader, useDocumentTitle } from '../futon';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';
import type { aboutQuery } from './__generated__/aboutQuery.graphql';

export const Route = createFileRoute('/about')({
  component: AboutPage,
});

const query = graphql`
  query aboutQuery {
    version {
      version
      revision
    }
    goModuleLicenses {
      path
      version
      files {
        name
        text
      }
    }
  }
`;

// AboutPage は、Kotatsu の版とライセンス、同梱するソフトウェアとそのライセンスの文書を出す (設計書 10.3、付録 B、#1604)。
function AboutPage() {
  const { t } = useTranslation();
  useDocumentTitle([t('about.title')]);

  return (
    <Stack gap="lg" maw={960}>
      <Title order={2}>{t('about.title')}</Title>
      <ErrorBoundary fallback={() => <Alert color="red">{t('about.loadFailed')}</Alert>}>
        <Suspense fallback={<PageLoader />}>
          <AboutContent />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

function AboutContent() {
  const { t } = useTranslation();
  const data = useLazyLoadQuery<aboutQuery>(query, {});
  const revision = data.version.revision ?? null;
  const [filter, setFilter] = useState('');
  // undefined は読んでいる途中、null は書き出していないビルド。
  const [bundled, setBundled] = useState<SoftwareEntry[] | null | undefined>(undefined);
  const [libaribcaptionNotice, setLibaribcaptionNotice] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    void fetchBundledPackages(t('about.generatedLicense'))
      .catch(() => null)
      .then((entries) => {
        if (active) {
          setBundled(entries);
        }
      });
    void fetch(`${import.meta.env.BASE_URL}licenses/libaribcaption.txt`)
      .then(async (response) => (response.ok ? response.text() : null))
      .catch(() => null)
      .then((text) => {
        if (active) {
          setLibaribcaptionNotice(text);
        }
      });

    return () => {
      active = false;
    };
  }, [t]);

  const goModules: SoftwareEntry[] | null =
    data.goModuleLicenses?.map((goModule) => ({
      // Go のモジュールは SPDX の名前を持たないので、ライセンスは文書だけで示す。
      name: goModule.path === 'std' ? t('about.goStandardLibrary') : goModule.path,
      version: goModule.version,
      license: '',
      url: goModule.path === 'std' ? 'https://go.dev/' : `https://pkg.go.dev/${goModule.path}`,
      licenses: goModule.files.map((file) => ({ name: file.name, text: file.text })),
      note: null,
    })) ?? null;
  const dataEntries = bundledData(libaribcaptionNotice, revision, {
    inRepository: t('about.inRepository'),
    inImage: t('about.inImage'),
  });
  const programs = imagePrograms(t('about.inImage'));

  return (
    <Stack gap="lg">
      <Card withBorder>
        <Stack gap="xs">
          <Text fw={600}>Kotatsu</Text>
          <Text size="sm">{t('about.version', { version: data.version.version })}</Text>
          <Text size="sm">
            {t('about.license')}{' '}
            <Anchor href={sourceURL('LICENSE', revision)} target="_blank" rel="noopener noreferrer">
              Apache License 2.0
            </Anchor>
          </Text>
          <Anchor href={repositoryURL} target="_blank" rel="noopener noreferrer" size="sm">
            {repositoryURL}
          </Anchor>
        </Stack>
      </Card>

      <Text size="sm">{t('about.description')}</Text>
      <TextInput
        label={t('about.filter')}
        placeholder={t('about.filterPlaceholder')}
        value={filter}
        onChange={(event) => setFilter(event.currentTarget.value)}
      />

      <Section title={t('about.frontend')} description={t('about.frontendDescription')}>
        {bundled === undefined ? (
          <PageLoader />
        ) : bundled === null ? (
          <Alert color="gray">{t('about.notGenerated')}</Alert>
        ) : (
          <LicenseList entries={filterEntries(bundled, filter)} />
        )}
      </Section>

      <Section title={t('about.backend')} description={t('about.backendDescription')}>
        {goModules === null ? (
          <Alert color="gray">{t('about.notGenerated')}</Alert>
        ) : (
          <LicenseList entries={filterEntries(goModules, filter)} />
        )}
      </Section>

      <Section title={t('about.data')} description={t('about.dataDescription')}>
        <LicenseList entries={filterEntries(dataEntries, filter)} />
      </Section>

      <Section title={t('about.image')} description={t('about.imageDescription')}>
        <LicenseList entries={filterEntries(programs, filter)} />
      </Section>
    </Stack>
  );
}

function Section({
  title,
  description,
  children,
}: {
  title: string;
  description: string;
  children: React.ReactNode;
}) {
  return (
    <Stack gap="xs">
      <Title order={3} size="h4">
        {title}
      </Title>
      <Text size="sm" c="dimmed">
        {description}
      </Text>
      {children}
    </Stack>
  );
}
