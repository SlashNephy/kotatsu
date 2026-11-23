import { describe, expect, test } from 'vitest';

import { crc32, streamTypes, TsDemuxer, tsPacketSize, type TsPes, type TsProgram } from './tsDemux';

// withCrc は、セクションの末尾に CRC-32 を付ける。
function withCrc(body: number[]): Uint8Array {
  const section = new Uint8Array(body.length + 4);
  section.set(body);
  const crc = crc32(section.subarray(0, body.length));
  section.set(
    [(crc >>> 24) & 0xff, (crc >>> 16) & 0xff, (crc >>> 8) & 0xff, crc & 0xff],
    body.length,
  );

  return section;
}

// sectionHeader は、table_id から last_section_number までの 8 バイト。section_length は CRC を含めて数える。
function sectionHeader(tableId: number, id: number, version: number, rest: number): number[] {
  const length = 5 + rest + 4;

  return [
    tableId,
    0xb0 | ((length >> 8) & 0x0f),
    length & 0xff,
    (id >> 8) & 0xff,
    id & 0xff,
    0xc1 | ((version & 0x1f) << 1),
    0,
    0,
  ];
}

function pat(programs: { serviceId: number; pmtPid: number }[]): Uint8Array {
  const entries = programs.flatMap(({ serviceId, pmtPid }) => [
    (serviceId >> 8) & 0xff,
    serviceId & 0xff,
    0xe0 | ((pmtPid >> 8) & 0x1f),
    pmtPid & 0xff,
  ]);

  return withCrc([...sectionHeader(0x00, 1, 0, entries.length), ...entries]);
}

type EsEntry = { streamType: number; pid: number; componentTag?: number };

function pmt(serviceId: number, streams: EsEntry[], version = 0): Uint8Array {
  // PCR_PID は 0x101、program_info は空。
  const body = [0xe0 | 0x01, 0x01, 0xf0, 0x00];
  for (const { streamType, pid, componentTag } of streams) {
    const descriptors = componentTag === undefined ? [] : [0x52, 1, componentTag];
    body.push(
      streamType,
      0xe0 | ((pid >> 8) & 0x1f),
      pid & 0xff,
      0xf0 | ((descriptors.length >> 8) & 0x0f),
      descriptors.length & 0xff,
      ...descriptors,
    );
  }

  return withCrc([...sectionHeader(0x02, serviceId, version, body.length), ...body]);
}

function timestamp(prefix: number, value: number): number[] {
  return [
    (prefix << 4) | ((Math.floor(value / 2 ** 30) & 0x07) << 1) | 1,
    Math.floor(value / 2 ** 22) & 0xff,
    ((Math.floor(value / 2 ** 15) & 0x7f) << 1) | 1,
    Math.floor(value / 2 ** 7) & 0xff,
    ((value & 0x7f) << 1) | 1,
  ];
}

function pes(streamId: number, payload: number[], pts: number, dts?: number): Uint8Array {
  const header =
    dts === undefined ? timestamp(0x2, pts) : [...timestamp(0x3, pts), ...timestamp(0x1, dts)];
  const flags = dts === undefined ? 0x80 : 0xc0;

  return new Uint8Array([
    0,
    0,
    1,
    streamId,
    0,
    0,
    0x80,
    flags,
    header.length,
    ...header,
    ...payload,
  ]);
}

// packets は、data を pid の TS パケットに分ける。セクションなら pointer_field を付ける。
function packets(
  pid: number,
  data: Uint8Array,
  { section = false, counter = { value: 0 } } = {},
): Uint8Array[] {
  const out: Uint8Array[] = [];
  let offset = 0;
  let first = true;
  while (offset < data.length || first) {
    const packet = new Uint8Array(tsPacketSize).fill(0xff);
    packet[0] = 0x47;
    packet[1] = (first ? 0x40 : 0) | ((pid >> 8) & 0x1f);
    packet[2] = pid & 0xff;
    let at = 4;
    if (section && first) {
      packet[at] = 0;
      at += 1;
    }
    const room = tsPacketSize - at;
    const chunk = data.subarray(offset, offset + room);
    if (chunk.length < room && !section) {
      // PES の最後のパケットは、アダプテーションフィールドで埋める。
      const stuffing = room - chunk.length;
      packet[3] = 0x30 | (counter.value & 0x0f);
      packet[4] = stuffing - 1;
      if (stuffing > 1) {
        packet[5] = 0x00;
      }
      packet.set(chunk, tsPacketSize - chunk.length);
    } else {
      packet[3] = 0x10 | (counter.value & 0x0f);
      packet.set(chunk, at);
    }
    counter.value += 1;
    out.push(packet);
    offset += chunk.length;
    first = false;
  }

  return out;
}

function concat(list: Uint8Array[]): Uint8Array {
  const out = new Uint8Array(list.reduce((sum, item) => sum + item.length, 0));
  let offset = 0;
  for (const item of list) {
    out.set(item, offset);
    offset += item.length;
  }

  return out;
}

function demux(serviceId: number | null) {
  const programs: TsProgram[] = [];
  const pesList: TsPes[] = [];
  const demuxer = new TsDemuxer({
    serviceId,
    onProgram: (program) => programs.push(program),
    onPes: (item) => pesList.push(item),
  });

  return { demuxer, programs, pesList };
}

const streams: EsEntry[] = [
  { streamType: streamTypes.mpeg2Video, pid: 0x100 },
  { streamType: streamTypes.aacAdts, pid: 0x110 },
  { streamType: streamTypes.aacAdts, pid: 0x111 },
  { streamType: streamTypes.privatePes, pid: 0x130, componentTag: 0x30 },
  { streamType: streamTypes.privatePes, pid: 0x138, componentTag: 0x38 },
];

describe('TsDemuxer', () => {
  test('PAT と PMT から番組を選び、映像、音声、字幕、文字スーパーを見分ける', () => {
    const { demuxer, programs } = demux(1024);
    demuxer.push(
      concat([
        ...packets(
          0,
          pat([
            { serviceId: 1032, pmtPid: 0x1f1 },
            { serviceId: 1024, pmtPid: 0x1f0 },
          ]),
          {
            section: true,
          },
        ),
        ...packets(0x1f1, pmt(1032, [{ streamType: streamTypes.mpeg2Video, pid: 0x200 }]), {
          section: true,
        }),
        ...packets(0x1f0, pmt(1024, streams), { section: true }),
      ]),
    );

    expect(programs).toEqual([
      {
        serviceId: 1024,
        pcrPid: 0x101,
        video: { pid: 0x100, streamType: 0x02, componentTag: null },
        audios: [
          { pid: 0x110, streamType: 0x0f, componentTag: null },
          { pid: 0x111, streamType: 0x0f, componentTag: null },
        ],
        captions: { pid: 0x130, streamType: 0x06, componentTag: 0x30 },
        superimpose: { pid: 0x138, streamType: 0x06, componentTag: 0x38 },
      },
    ]);
  });

  test('パケットの途中で切れて届いても、同期がずれても、PES を組み立てる', () => {
    const { demuxer, pesList } = demux(null);
    const video = Array.from({ length: 500 }, (_, i) => i & 0xff);
    // continuity_counter は、PES をまたいで続ける。
    const counter = { value: 0 };
    const stream = concat([
      ...packets(0, pat([{ serviceId: 1024, pmtPid: 0x1f0 }]), { section: true }),
      ...packets(0x1f0, pmt(1024, streams), { section: true }),
      new Uint8Array([0x00, 0x13]),
      ...packets(0x100, pes(0xe0, video, 9_000, 6_000), { counter }),
      ...packets(0x100, pes(0xe0, [1, 2, 3], 12_000), { counter }),
    ]);
    for (let i = 0; i < stream.length; i += 77) {
      demuxer.push(stream.subarray(i, i + 77));
    }
    demuxer.flush();

    expect(
      pesList.map(({ kind, pts, dts, data, discontinuous }) => ({
        kind,
        pts,
        dts,
        data: [...data],
        discontinuous,
      })),
    ).toEqual([
      { kind: 'video', pts: 9_000, dts: 6_000, data: video, discontinuous: false },
      { kind: 'video', pts: 12_000, dts: 12_000, data: [1, 2, 3], discontinuous: false },
    ]);
  });

  test('PMT を読む前に届いたパケットを取っておき、読んだら番組の分を流す', () => {
    const { demuxer, pesList } = demux(1024);
    demuxer.push(
      concat([
        ...packets(0x100, pes(0xe0, [7, 7, 7], 3_000)),
        ...packets(0x110, pes(0xc0, [8], 3_100)),
        ...packets(0, pat([{ serviceId: 1024, pmtPid: 0x1f0 }]), { section: true }),
        ...packets(0x1f0, pmt(1024, streams), { section: true }),
      ]),
    );
    demuxer.flush();

    expect(pesList.map(({ kind, pts }) => ({ kind, pts }))).toEqual([
      { kind: 'video', pts: 3_000 },
      { kind: 'audio', pts: 3_100 },
    ]);
  });

  test('CRC の合わない PMT は使わない', () => {
    const { demuxer, programs } = demux(1024);
    const broken = pmt(1024, streams);
    const last = broken.length - 1;
    broken[last] = broken[last]! ^ 0xff;
    demuxer.push(
      concat([
        ...packets(0, pat([{ serviceId: 1024, pmtPid: 0x1f0 }]), { section: true }),
        ...packets(0x1f0, broken, { section: true }),
      ]),
    );

    expect(programs).toEqual([]);
  });

  test('パケットが欠けた PES は、欠けたと印を付けて渡し、頭を逃した PES は捨てる', () => {
    const { demuxer, pesList } = demux(1024);
    const counter = { value: 0 };
    const video = packets(
      0x100,
      pes(
        0xe0,
        Array.from({ length: 600 }, () => 1),
        1_000,
      ),
      {
        counter,
      },
    );
    const tail = packets(
      0x100,
      pes(
        0xe0,
        Array.from({ length: 400 }, () => 2),
        2_000,
      ),
      {
        counter: { value: 5 },
      },
    );
    demuxer.push(
      concat([
        ...packets(0, pat([{ serviceId: 1024, pmtPid: 0x1f0 }]), { section: true }),
        ...packets(0x1f0, pmt(1024, streams), { section: true }),
        // 2 つ目のパケットが欠ける。
        video[0]!,
        ...video.slice(2),
        // 次の PES は、頭のパケットが欠ける。
        ...tail.slice(1),
      ]),
    );
    demuxer.flush();

    expect(pesList.map(({ pts, discontinuous }) => ({ pts, discontinuous }))).toEqual([
      { pts: 1_000, discontinuous: true },
    ]);
  });

  test('文字スーパー (private_stream_2) の PES は、時刻を持たないまま、ペイロードを渡す', () => {
    const { demuxer, pesList } = demux(1024);
    // private_stream_2 は、PES_packet_length の後にすぐペイロードが続く。
    const superimpose = new Uint8Array([0, 0, 1, 0xbf, 0, 4, 0x81, 0xff, 0xf0, 0x01]);
    demuxer.push(
      concat([
        ...packets(0, pat([{ serviceId: 1024, pmtPid: 0x1f0 }]), { section: true }),
        ...packets(0x1f0, pmt(1024, streams), { section: true }),
        ...packets(0x138, superimpose),
      ]),
    );
    demuxer.flush();

    expect(pesList.map(({ kind, pts, data }) => ({ kind, pts, data: [...data] }))).toEqual([
      { kind: 'superimpose', pts: null, data: [0x81, 0xff, 0xf0, 0x01] },
    ]);
  });

  test('PES_packet_length の分がそろったら、次の頭を待たずに渡す', () => {
    const { demuxer, pesList } = demux(1024);
    // 字幕の PES は、PES_packet_length を持つ。2 つのパケットにまたがる大きさにする。
    const caption = Array.from({ length: 300 }, (_, i) => i & 0xff);
    const data = pes(0xbd, caption, 9_000);
    data[4] = (data.length - 6) >> 8;
    data[5] = (data.length - 6) & 0xff;
    demuxer.push(
      concat([
        ...packets(0, pat([{ serviceId: 1024, pmtPid: 0x1f0 }]), { section: true }),
        ...packets(0x1f0, pmt(1024, streams), { section: true }),
        ...packets(0x130, data),
      ]),
    );

    // flush も、次の PES の頭も送らない。
    expect(pesList.map(({ kind, pts, data }) => ({ kind, pts, data: [...data] }))).toEqual([
      { kind: 'captions', pts: 9_000, data: caption },
    ]);
  });

  test('重複して送られたパケットは、二重につながない', () => {
    const { demuxer, pesList } = demux(1024);
    const video = packets(
      0x100,
      pes(
        0xe0,
        Array.from({ length: 300 }, (_, i) => i & 0xff),
        1_000,
      ),
    );
    demuxer.push(
      concat([
        ...packets(0, pat([{ serviceId: 1024, pmtPid: 0x1f0 }]), { section: true }),
        ...packets(0x1f0, pmt(1024, streams), { section: true }),
        video[0]!,
        video[1]!,
        // 同じ continuity_counter のパケットが、もう一度届く。
        video[1]!,
      ]),
    );
    demuxer.flush();

    expect(
      pesList.map(({ data, discontinuous }) => ({ length: data.length, discontinuous })),
    ).toEqual([{ length: 300, discontinuous: false }]);
  });

  test('PMT の版が変わったら、番組を知らせ直す', () => {
    const { demuxer, programs } = demux(1024);
    demuxer.push(
      concat([
        ...packets(0, pat([{ serviceId: 1024, pmtPid: 0x1f0 }]), { section: true }),
        ...packets(0x1f0, pmt(1024, streams, 0), { section: true }),
        ...packets(0x1f0, pmt(1024, streams, 0), { section: true }),
        ...packets(0x1f0, pmt(1024, streams.slice(0, 2), 1), { section: true }),
      ]),
    );

    expect(programs.map((program) => program.audios.length)).toEqual([2, 1]);
  });
});
