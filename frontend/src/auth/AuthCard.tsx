import { Card, Center, Stack, Text, Title } from '@mantine/core';
import type { ReactNode } from 'react';

type Props = {
  title: string;
  description?: string;
  children: ReactNode;
};

// AuthCard は、セットアップとログインの画面の枠。
export function AuthCard({ title, description, children }: Props) {
  return (
    <Center py="xl">
      <Card withBorder w="100%" maw={440} padding="lg">
        <Stack>
          <Title order={2} ta="center">
            {title}
          </Title>
          {description !== undefined && (
            <Text size="sm" c="dimmed">
              {description}
            </Text>
          )}
          {children}
        </Stack>
      </Card>
    </Center>
  );
}
