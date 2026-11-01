import { describe, expect, test } from 'vitest';

import { captureMetadata, type CaptureInfo } from './upload';

const live: CaptureInfo = {
  time: Date.UTC(2026, 9, 10, 3, 0, 0),
  takenAt: Date.UTC(2026, 9, 10, 3, 0, 5),
  serviceName: 'ＮＨＫ総合',
  programName: 'ニュース',
  recordingId: null,
  position: null,
  caption: 'こんにちは',
};

describe('captureMetadata', () => {
  test.each([
    {
      name: 'ライブでは、録画と再生位置を送らない',
      info: { ...live, position: 12 },
      want: { recordingId: null, position: null },
    },
    {
      name: '録画では、録画と再生位置を送る',
      info: { ...live, recordingId: 'UmVjb3JkaW5nOjE', position: 12.5 },
      want: { recordingId: 'UmVjb3JkaW5nOjE', position: 12.5 },
    },
  ])('$name', ({ info, want }) => {
    expect(captureMetadata(info)).toEqual({
      serviceName: 'ＮＨＫ総合',
      programName: 'ニュース',
      caption: 'こんにちは',
      broadcastAt: '2026-10-10T03:00:00.000Z',
      takenAt: '2026-10-10T03:00:05.000Z',
      ...want,
    });
  });
});
