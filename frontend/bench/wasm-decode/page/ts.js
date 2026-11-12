// 最小の TS → PES。最初の番組の映像 (0x02) と音声 (0x0f) を拾う。
export function demux(buf) {
  const u8 = new Uint8Array(buf);
  let pmtPid = -1,
    vpid = -1,
    apid = -1;
  const pending = new Map(); // pid -> { chunks, size }
  const video = [],
    audio = [];
  let cc = new Map(),
    ccErrors = 0;
  const finish = (pid) => {
    const p = pending.get(pid);
    if (!p || p.size === 0) return;
    const pes = new Uint8Array(p.size);
    let o = 0;
    for (const c of p.chunks) {
      pes.set(c, o);
      o += c.length;
    }
    pending.delete(pid);
    if (pes[0] !== 0 || pes[1] !== 0 || pes[2] !== 1) return;
    const flags = pes[7],
      hlen = pes[8];
    let pts = -1;
    if (flags & 0x80) {
      pts =
        ((pes[9] >> 1) & 7) * 2 ** 30 +
        ((pes[10] << 7) | (pes[11] >> 1)) * 2 ** 15 +
        ((pes[12] << 7) | (pes[13] >> 1));
    }
    const payload = pes.subarray(9 + hlen);
    (pid === vpid ? video : audio).push({ pts, data: payload });
  };
  for (let i = 0; i + 188 <= u8.length; i += 188) {
    if (u8[i] !== 0x47) continue;
    const pusi = u8[i + 1] & 0x40;
    const pid = ((u8[i + 1] & 0x1f) << 8) | u8[i + 2];
    const afc = (u8[i + 3] >> 4) & 3;
    const c = u8[i + 3] & 0xf;
    let off = i + 4;
    if (afc & 2) off += 1 + u8[i + 4];
    if (!(afc & 1) || off >= i + 188) continue;
    if (cc.has(pid) && ((cc.get(pid) + 1) & 0xf) !== c) ccErrors++;
    cc.set(pid, c);
    const pl = u8.subarray(off, i + 188);
    if (pid === 0 && pusi) {
      const s = pl.subarray(1 + pl[0]);
      pmtPid = ((s[10] & 0x1f) << 8) | s[11];
      if (((s[8] << 8) | s[9]) === 0) pmtPid = ((s[14] & 0x1f) << 8) | s[15];
      continue;
    }
    if (pid === pmtPid && pusi && vpid < 0) {
      const s = pl.subarray(1 + pl[0]);
      const len = ((s[1] & 0xf) << 8) | s[2];
      const pil = ((s[10] & 0xf) << 8) | s[11];
      for (let j = 12 + pil; j < 3 + len - 4;) {
        const st = s[j],
          ep = ((s[j + 1] & 0x1f) << 8) | s[j + 2],
          el = ((s[j + 3] & 0xf) << 8) | s[j + 4];
        if (st === 0x02 && vpid < 0) vpid = ep;
        if (st === 0x0f && apid < 0) apid = ep;
        j += 5 + el;
      }
      continue;
    }
    if (pid !== vpid && pid !== apid) continue;
    if (pusi) finish(pid);
    let p = pending.get(pid);
    if (!p) {
      if (!pusi) continue;
      p = { chunks: [], size: 0 };
      pending.set(pid, p);
    }
    p.chunks.push(pl);
    p.size += pl.length;
  }
  finish(vpid);
  finish(apid);
  return { video, audio, ccErrors };
}
