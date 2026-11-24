// ブラウザ内でデコードする経路 (#1516) の、TS を PES に分ける部分。
// 届いた順に少しずつ受け取り (パケットの途中で切れていてもよい)、PAT と PMT から番組を選び、
// 映像 (MPEG-2)、音声 (AAC の ADTS)、字幕と文字スーパーの PES を組み立てて渡す。
// デコードはしない。PTS は 33 ビットのまま渡す (伸ばすのは ptsTimeline.ts)。

// tsPacketSize は、TS のパケットの大きさ。
export const tsPacketSize = 188;
const syncByte = 0x47;

// streamTypes は、PMT の stream_type のうち、扱うもの。
export const streamTypes = {
  mpeg2Video: 0x02,
  aacAdts: 0x0f,
  privatePes: 0x06,
} as const;

// ARIB の字幕と文字スーパーは、PMT の stream_type 0x06 で、ストリーム識別記述子 (0x52) の component_tag で見分ける
// (ARIB TR-B14)。字幕は 0x30〜0x37、文字スーパーは 0x38〜0x3F。
const streamIdentifierDescriptor = 0x52;
const captionTags = { from: 0x30, to: 0x37 };
const superimposeTags = { from: 0x38, to: 0x3f };

export type TsStreamKind = 'video' | 'audio' | 'captions' | 'superimpose';

// TsStream は、PMT に載った ES のうち、扱うもの。
export type TsStream = {
  pid: number;
  streamType: number;
  // componentTag は、ストリーム識別記述子の component_tag。なければ null。
  componentTag: number | null;
};

// TsProgram は、選んだ番組の ES。音声は、PMT に載った順に並べる。
export type TsProgram = {
  serviceId: number;
  pcrPid: number;
  video: TsStream | null;
  audios: TsStream[];
  captions: TsStream | null;
  superimpose: TsStream | null;
};

// TsPes は、組み立てた PES。data は PES のヘッダーを除いたペイロード。
export type TsPes = {
  kind: TsStreamKind;
  pid: number;
  // pts と dts は 33 ビットのまま。なければ null。dts がなければ pts と同じ。
  pts: number | null;
  dts: number | null;
  data: Uint8Array;
  // discontinuous は、組み立てる間に continuity_counter が飛んだか (パケットが欠けたか)。欠けていても渡し、
  // 使うかはデコードの側で決める。
  discontinuous: boolean;
};

export type TsDemuxerOptions = {
  // serviceId は、選ぶ番組の program_number。null なら PAT の最初の番組。
  serviceId: number | null;
  onProgram: (program: TsProgram) => void;
  onPes: (pes: TsPes) => void;
  // pendingLimit は、PMT を読むまでに取っておくパケットの数の上限。超えたら古いものから破棄する。
  pendingLimit?: number;
};

// defaultPendingLimit は、PMT を読むまでに取っておくパケットの数の既定。
// 24 Mbps の TS で約 1 秒分。PMT は 100ms 程度の間隔で届くので、足りる。
export const defaultPendingLimit = 16_000;

type SectionBuffer = { data: Uint8Array; length: number; expected: number };

type PesBuffer = {
  stream: TsStream;
  kind: TsStreamKind;
  chunks: Uint8Array[];
  size: number;
  // expected は、PES_packet_length から分かる PES の大きさ。0 (映像) なら null で、次の PES の先頭まで組み立てる。
  expected: number | null;
  discontinuous: boolean;
};

// TsDemuxer は、TS を受け取って、選んだ番組の PES を渡す。
export class TsDemuxer {
  private readonly options: Required<TsDemuxerOptions>;
  // carry は、前に受け取った分の、パケットに満たない端。
  private carry = new Uint8Array(0);
  private pmtPid: number | null = null;
  private program: TsProgram | null = null;
  private pmtVersion: number | null = null;
  private readonly sections = new Map<number, SectionBuffer>();
  private readonly pes = new Map<number, PesBuffer>();
  private readonly continuity = new Map<number, number>();
  private readonly streamsByPid = new Map<number, { stream: TsStream; kind: TsStreamKind }>();
  // pending は、PMT を読むまでに届いたパケット。読んだら、番組の分を順に流す。
  private pending: Uint8Array[] = [];

  constructor(options: TsDemuxerOptions) {
    this.options = { pendingLimit: defaultPendingLimit, ...options };
  }

  // push は、TS の続きを受け取る。パケットの途中で切れていてもよい。
  push(chunk: Uint8Array): void {
    let data = chunk;
    if (this.carry.length > 0) {
      data = new Uint8Array(this.carry.length + chunk.length);
      data.set(this.carry);
      data.set(chunk, this.carry.length);
    }
    let offset = 0;
    while (data.length - offset >= tsPacketSize) {
      if (data[offset] !== syncByte) {
        // 同期がずれたら、次の同期バイトを探す。
        offset += 1;
        continue;
      }
      this.packet(data.subarray(offset, offset + tsPacketSize));
      offset += tsPacketSize;
    }
    this.carry = data.slice(offset);
  }

  // flush は、組み立て中の PES を渡す。読み終えたときに呼ぶ。
  flush(): void {
    // Map は、回している間に消したキーを飛ばすので、そのまま回して消してよい。
    for (const pid of this.pes.keys()) {
      this.finishPes(pid);
    }
  }

  // reset は、読んだものをすべて忘れる。選局したとき (serviceId を替えるとき) に呼ぶ。
  reset(serviceId: number | null): void {
    this.options.serviceId = serviceId;
    this.carry = new Uint8Array(0);
    this.pmtPid = null;
    this.program = null;
    this.pmtVersion = null;
    this.sections.clear();
    this.pes.clear();
    this.continuity.clear();
    this.streamsByPid.clear();
    this.pending = [];
  }

  private packet(packet: Uint8Array): void {
    const pid = ((packet[1]! & 0x1f) << 8) | packet[2]!;
    if (pid === 0x1fff) {
      return;
    }
    // transport_error_indicator が立ったパケットは、内容を信じない。
    if (packet[1]! & 0x80) {
      this.markDiscontinuous(pid);

      return;
    }
    if (pid === 0) {
      this.section(pid, packet);

      return;
    }
    if (pid === this.pmtPid) {
      this.section(pid, packet);

      return;
    }
    if (this.program === null) {
      // PMT を読む前のパケットは取っておき、読んだら番組の分を流す。破棄すると、PMT の前から始まっていた
      // I フレームが失われ、次の GOP まで映らない (#1516)。
      this.pending.push(packet.slice());
      if (this.pending.length > this.options.pendingLimit) {
        this.pending.shift();
      }

      return;
    }
    this.elementary(pid, packet);
  }

  private markDiscontinuous(pid: number): void {
    const buffer = this.pes.get(pid);
    if (buffer !== undefined) {
      buffer.discontinuous = true;
    }
    this.continuity.delete(pid);
  }

  // payloadOf は、パケットのペイロードを返す。ペイロードがなければ null。
  private payloadOf(packet: Uint8Array): Uint8Array | null {
    const adaptation = (packet[3]! >> 4) & 0x3;
    let offset = 4;
    if (adaptation & 0x2) {
      offset += 1 + packet[4]!;
    }
    if (!(adaptation & 0x1) || offset >= tsPacketSize) {
      return null;
    }

    return packet.subarray(offset);
  }

  // continuity は、continuity_counter から、続いているか、欠けたか、重複して送られたかを返し、保持し直す。
  // ペイロードのあるパケットだけを数える。同じ値の繰り返しは、重複して送られたパケット (ISO/IEC 13818-1 2.4.3.3) で、破棄する。
  private continuityOf(pid: number, packet: Uint8Array): 'continuous' | 'lost' | 'duplicate' {
    const counter = packet[3]! & 0xf;
    const previous = this.continuity.get(pid);
    this.continuity.set(pid, counter);
    if (previous === undefined || counter === ((previous + 1) & 0xf)) {
      return 'continuous';
    }

    return counter === previous ? 'duplicate' : 'lost';
  }

  private section(pid: number, packet: Uint8Array): void {
    const payload = this.payloadOf(packet);
    if (payload === null) {
      return;
    }
    const start = (packet[1]! & 0x40) !== 0;
    let body = payload;
    if (start) {
      const pointer = payload[0]!;
      const previous = this.sections.get(pid);
      if (previous !== undefined) {
        this.appendSection(pid, previous, payload.subarray(1, 1 + pointer));
      }
      body = payload.subarray(1 + pointer);
      this.sections.delete(pid);
      // 1 つのパケットに、セクションが続けて入ることがある。
      while (body.length >= 3 && body[0] !== 0xff) {
        const expected = 3 + (((body[1]! & 0x0f) << 8) | body[2]!);
        const buffer: SectionBuffer = { data: new Uint8Array(expected), length: 0, expected };
        const taken = Math.min(expected, body.length);
        buffer.data.set(body.subarray(0, taken));
        buffer.length = taken;
        body = body.subarray(taken);
        if (buffer.length === expected) {
          this.completeSection(pid, buffer.data);
        } else {
          this.sections.set(pid, buffer);
        }
      }

      return;
    }
    const buffer = this.sections.get(pid);
    if (buffer !== undefined) {
      this.appendSection(pid, buffer, body);
    }
  }

  private appendSection(pid: number, buffer: SectionBuffer, data: Uint8Array): void {
    const taken = Math.min(buffer.expected - buffer.length, data.length);
    buffer.data.set(data.subarray(0, taken), buffer.length);
    buffer.length += taken;
    if (buffer.length === buffer.expected) {
      this.sections.delete(pid);
      this.completeSection(pid, buffer.data);
    }
  }

  private completeSection(pid: number, section: Uint8Array): void {
    if (section.length < 12 || crc32(section) !== 0) {
      return;
    }
    // current_next_indicator が 0 のセクションは、これから使うものなので、まだ使わない。
    if ((section[5]! & 0x01) === 0) {
      return;
    }
    if (pid === 0 && section[0] === 0x00) {
      this.pat(section);
    } else if (pid === this.pmtPid && section[0] === 0x02) {
      this.pmt(section);
    }
  }

  private pat(section: Uint8Array): void {
    const end = section.length - 4;
    for (let i = 8; i + 4 <= end; i += 4) {
      const number = (section[i]! << 8) | section[i + 1]!;
      const pid = ((section[i + 2]! & 0x1f) << 8) | section[i + 3]!;
      // program_number 0 は、NIT の PID。
      if (number === 0) {
        continue;
      }
      if (this.options.serviceId === null || this.options.serviceId === number) {
        if (this.pmtPid !== pid) {
          this.pmtPid = pid;
          this.pmtVersion = null;
          this.sections.delete(pid);
        }

        return;
      }
    }
  }

  private pmt(section: Uint8Array): void {
    const serviceId = (section[3]! << 8) | section[4]!;
    if (this.options.serviceId !== null && serviceId !== this.options.serviceId) {
      return;
    }
    const version = (section[5]! >> 1) & 0x1f;
    if (version === this.pmtVersion) {
      return;
    }
    this.pmtVersion = version;
    const pcrPid = ((section[8]! & 0x1f) << 8) | section[9]!;
    const infoLength = ((section[10]! & 0x0f) << 8) | section[11]!;
    const end = section.length - 4;
    const program: TsProgram = {
      serviceId,
      pcrPid,
      video: null,
      audios: [],
      captions: null,
      superimpose: null,
    };
    for (let i = 12 + infoLength; i + 5 <= end;) {
      const streamType = section[i]!;
      const pid = ((section[i + 1]! & 0x1f) << 8) | section[i + 2]!;
      const length = ((section[i + 3]! & 0x0f) << 8) | section[i + 4]!;
      const stream: TsStream = {
        pid,
        streamType,
        componentTag: componentTagOf(section.subarray(i + 5, Math.min(i + 5 + length, end))),
      };
      if (streamType === streamTypes.mpeg2Video && program.video === null) {
        program.video = stream;
      } else if (streamType === streamTypes.aacAdts) {
        program.audios.push(stream);
      } else if (streamType === streamTypes.privatePes && stream.componentTag !== null) {
        const tag = stream.componentTag;
        if (tag >= captionTags.from && tag <= captionTags.to && program.captions === null) {
          program.captions = stream;
        } else if (
          tag >= superimposeTags.from &&
          tag <= superimposeTags.to &&
          program.superimpose === null
        ) {
          program.superimpose = stream;
        }
      }
      i += 5 + length;
    }
    const first = this.program === null;
    this.program = program;
    this.streamsByPid.clear();
    if (program.video !== null) {
      this.streamsByPid.set(program.video.pid, { stream: program.video, kind: 'video' });
    }
    for (const audio of program.audios) {
      this.streamsByPid.set(audio.pid, { stream: audio, kind: 'audio' });
    }
    if (program.captions !== null) {
      this.streamsByPid.set(program.captions.pid, { stream: program.captions, kind: 'captions' });
    }
    if (program.superimpose !== null) {
      this.streamsByPid.set(program.superimpose.pid, {
        stream: program.superimpose,
        kind: 'superimpose',
      });
    }
    // 番組から外れた ES の、組み立て中の PES は破棄する。
    for (const pid of this.pes.keys()) {
      if (!this.streamsByPid.has(pid)) {
        this.pes.delete(pid);
      }
    }
    this.options.onProgram(program);
    if (first) {
      const pending = this.pending;
      this.pending = [];
      for (const packet of pending) {
        const pid = ((packet[1]! & 0x1f) << 8) | packet[2]!;
        this.elementary(pid, packet);
      }
    }
  }

  private elementary(pid: number, packet: Uint8Array): void {
    const entry = this.streamsByPid.get(pid);
    if (entry === undefined) {
      return;
    }
    const payload = this.payloadOf(packet);
    if (payload === null) {
      return;
    }
    const continuity = this.continuityOf(pid, packet);
    if (continuity === 'duplicate') {
      return;
    }
    const start = (packet[1]! & 0x40) !== 0;
    if (start) {
      // 頭のパケットの前が欠けていたら、組み立て中の PES は尾が欠けている。
      const previous = this.pes.get(pid);
      if (previous !== undefined && continuity === 'lost') {
        previous.discontinuous = true;
      }
      this.finishPes(pid);
      const length = payload.length >= 6 ? (payload[4]! << 8) | payload[5]! : 0;
      this.pes.set(pid, {
        stream: entry.stream,
        kind: entry.kind,
        chunks: [payload.slice()],
        size: payload.length,
        expected: length > 0 ? 6 + length : null,
        discontinuous: false,
      });
      this.finishIfComplete(pid);

      return;
    }
    const buffer = this.pes.get(pid);
    if (buffer === undefined) {
      // PES の先頭を逃したパケットは、組み立てられないので破棄する。
      return;
    }
    if (continuity === 'lost') {
      buffer.discontinuous = true;
    }
    buffer.chunks.push(payload.slice());
    buffer.size += payload.length;
    this.finishIfComplete(pid);
  }

  // finishIfComplete は、PES_packet_length の分がそろった PES を、次の PES の先頭を待たずに渡す。
  // 字幕と文字スーパーの PES は約 1 秒おきに届くので、次の PES の先頭を待つと、出すのがその分遅れる。
  private finishIfComplete(pid: number): void {
    const buffer = this.pes.get(pid);
    if (buffer !== undefined && buffer.expected !== null && buffer.size >= buffer.expected) {
      this.finishPes(pid);
    }
  }

  private finishPes(pid: number): void {
    const buffer = this.pes.get(pid);
    if (buffer === undefined) {
      return;
    }
    this.pes.delete(pid);
    const whole = new Uint8Array(buffer.size);
    let offset = 0;
    for (const chunk of buffer.chunks) {
      whole.set(chunk, offset);
      offset += chunk.length;
    }
    // PES_packet_length より後ろは、パケットの詰め物なので外す。
    const packet =
      buffer.expected !== null && buffer.expected < whole.length
        ? whole.subarray(0, buffer.expected)
        : whole;
    const parsed = parsePes(packet);
    if (parsed === null) {
      return;
    }
    this.options.onPes({
      kind: buffer.kind,
      pid,
      pts: parsed.pts,
      dts: parsed.dts,
      data: parsed.data,
      discontinuous: buffer.discontinuous,
    });
  }
}

// componentTagOf は、ES の記述子から、ストリーム識別記述子の component_tag を返す。なければ null。
function componentTagOf(descriptors: Uint8Array): number | null {
  for (let i = 0; i + 2 <= descriptors.length;) {
    const tag = descriptors[i]!;
    const length = descriptors[i + 1]!;
    if (tag === streamIdentifierDescriptor && length >= 1 && i + 2 < descriptors.length) {
      return descriptors[i + 2]!;
    }
    i += 2 + length;
  }

  return null;
}

// readTimestamp は、PES のヘッダーの 5 バイトの時刻 (33 ビット) を読む。
function readTimestamp(data: Uint8Array, offset: number): number {
  return (
    ((data[offset]! >> 1) & 0x07) * 2 ** 30 +
    ((data[offset + 1]! << 7) | (data[offset + 2]! >> 1)) * 2 ** 15 +
    ((data[offset + 3]! << 7) | (data[offset + 4]! >> 1))
  );
}

// headerlessStreamIds は、PES のヘッダーの続き (フラグと時刻) を持たない stream_id (ISO/IEC 13818-1 表 2-22)。
// ARIB の文字スーパーは private_stream_2 (0xBF) で届き、時刻を持たない。
const headerlessStreamIds = new Set([0xbc, 0xbe, 0xbf, 0xf0, 0xf1, 0xf2, 0xf8, 0xff]);

// parsePes は、PES を、時刻とペイロードに分ける。PES でなければ null。
export function parsePes(
  packet: Uint8Array,
): { pts: number | null; dts: number | null; data: Uint8Array } | null {
  if (packet.length < 6 || packet[0] !== 0 || packet[1] !== 0 || packet[2] !== 1) {
    return null;
  }
  if (headerlessStreamIds.has(packet[3]!)) {
    return { pts: null, dts: null, data: packet.subarray(6) };
  }
  if (packet.length < 9) {
    return null;
  }
  const flags = packet[7]!;
  const headerLength = packet[8]!;
  if (9 + headerLength > packet.length) {
    return null;
  }
  let pts: number | null = null;
  let dts: number | null = null;
  if (flags & 0x80 && headerLength >= 5) {
    pts = readTimestamp(packet, 9);
    if (flags & 0x40 && headerLength >= 10) {
      dts = readTimestamp(packet, 14);
    }
  }

  return { pts, dts: dts ?? pts, data: packet.subarray(9 + headerLength) };
}

// crc32Table は、MPEG-2 の CRC-32 (多項式 0x04C11DB7、反転なし) のテーブル。
const crc32Table = (() => {
  const table = new Uint32Array(256);
  for (let i = 0; i < 256; i++) {
    let crc = i << 24;
    for (let bit = 0; bit < 8; bit++) {
      crc = crc & 0x80000000 ? (crc << 1) ^ 0x04c11db7 : crc << 1;
    }
    table[i] = crc >>> 0;
  }

  return table;
})();

// crc32 は、MPEG-2 の CRC-32 を返す。CRC を含めたセクション全体を渡すと、正しければ 0 になる。
export function crc32(data: Uint8Array): number {
  let crc = 0xffffffff;
  for (const byte of data) {
    crc = ((crc << 8) ^ crc32Table[((crc >>> 24) ^ byte) & 0xff]!) >>> 0;
  }

  return crc >>> 0;
}
