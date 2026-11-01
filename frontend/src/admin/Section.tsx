import { Card, Stack, Title } from '@mantine/core';
import type { ReactNode } from 'react';

// Section は、管理者向けの画面の、見出しの付いたまとまり。
export function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Card withBorder>
      <Stack gap="xs">
        <Title order={3} size="h5">
          {title}
        </Title>
        {children}
      </Stack>
    </Card>
  );
}
