import { MantineProvider } from '@mantine/core';
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { RelayEnvironmentProvider } from 'react-relay';
import type { GraphQLResponse } from 'relay-runtime';
import { afterEach, describe, expect, test } from 'vitest';

import { createTestEnvironment } from '../test/route';
import { LibraryArtifacts } from './LibraryArtifacts';

afterEach(cleanup);

// renderArtifacts は、libraryArtifacts に measured を返し、clearLibraryArtifacts に cleared を返す環境で、片付けの欄を出す。
// 送った mutation の数を返す関数を返す。
async function renderArtifacts(measured: GraphQLResponse, cleared?: GraphQLResponse) {
  let sent = 0;
  const environment = createTestEnvironment((request) => {
    if (request.operationKind === 'mutation') {
      sent++;

      return cleared;
    }

    return measured;
  });
  await act(async () => {
    render(
      <MantineProvider env="test">
        <RelayEnvironmentProvider environment={environment}>
          <LibraryArtifacts />
        </RelayEnvironmentProvider>
      </MantineProvider>,
    );
  });

  return () => sent;
}

const measured = { data: { libraryArtifacts: { size: 2.5e9, files: 1234 } } };

describe('LibraryArtifacts (#1896)', () => {
  test('押すと、消える量を数えて見せ、確かめてから消す', async () => {
    const sent = await renderArtifacts(measured, {
      data: { clearLibraryArtifacts: { size: 2.5e9, files: 1234 } },
    });
    expect(screen.queryByRole('dialog')).toBeNull();

    await act(async () => fireEvent.click(screen.getByRole('button', { name: '片付ける' })));
    expect(await screen.findByText(/1234 個のファイル \(2\.5 GB\) を消します/)).toBeDefined();
    expect(sent()).toBe(0);

    await act(async () => fireEvent.click(screen.getByRole('button', { name: '消す' })));
    expect(sent()).toBe(1);
    expect((await screen.findByRole('status')).textContent).toBe(
      '1234 個のファイル (2.5 GB) を消しました',
    );
  });

  test('キャンセルすれば消さない', async () => {
    const sent = await renderArtifacts(measured);
    await act(async () => fireEvent.click(screen.getByRole('button', { name: '片付ける' })));
    await screen.findByText(/1234 個のファイル/);

    await act(async () => fireEvent.click(screen.getByRole('button', { name: 'キャンセル' })));
    expect(sent()).toBe(0);
  });

  test('消すものがなければ、そう伝えて消すボタンを押せなくする', async () => {
    const sent = await renderArtifacts({ data: { libraryArtifacts: { size: 0, files: 0 } } });
    await act(async () => fireEvent.click(screen.getByRole('button', { name: '片付ける' })));

    expect(await screen.findByText('片付ける生成物はありません')).toBeDefined();
    expect(screen.getByRole('button', { name: '消す' })).toHaveProperty('disabled', true);
    expect(sent()).toBe(0);
  });

  test('データディレクトリがなければ、片付けられないことを伝える', async () => {
    await renderArtifacts({
      data: null,
      errors: [
        { message: 'the data directory is not configured', extensions: { code: 'CONFLICT' } },
      ],
    } as unknown as GraphQLResponse);
    await act(async () => fireEvent.click(screen.getByRole('button', { name: '片付ける' })));

    expect((await screen.findByRole('alert')).textContent).toBe(
      'データディレクトリを設定していないので、片付けられません',
    );
    expect(screen.queryByRole('dialog')).toBeNull();
  });
});
