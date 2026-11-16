// 受け取りの速さを測る (#1513)。届いた量を時刻とともに溜め、窓の中の速さを返す。

// Received は、届いた量。transferMs は、その塊を読むのにかかった時間 (HLS のセグメント)。分からなければ null。
type Received = { at: number; bytes: number; transferMs: number | null };

// ReceiveMeter は、届いた量から、受け取りの速さ (bps) を出す。時刻はミリ秒。
export class ReceiveMeter {
  #received: Received[] = [];
  #startedAt: number | null = null;
  #limitedAt = -Infinity;

  // add は、at に bytes が届いたことを知らせる。
  add(at: number, bytes: number, transferMs: number | null = null) {
    if (bytes <= 0) {
      return;
    }
    this.#startedAt ??= at;
    this.#received.push({ at, bytes, transferMs });
  }

  // limit は、at に、受け手が自分で読むのを絞っていた (先に十分読んである) ことを知らせる。
  // 絞っている間の速さは、回線の速さではないので、窓がその時刻を含む間は delivered が null を返す。
  limit(at: number) {
    this.#limitedAt = at;
  }

  reset() {
    this.#received = [];
    this.#startedAt = null;
    this.#limitedAt = -Infinity;
  }

  #prune(now: number, keepMs: number) {
    while (this.#received.length > 0 && now - this.#received[0]!.at > keepMs) {
      this.#received.shift();
    }
  }

  // delivered は、now までの windowMs のあいだに届いた量を、窓の長さで割った速さ。測り始めてから窓の長さがたっていないか、
  // 窓の中で読むのを絞っていたら null。
  delivered(now: number, windowMs: number): number | null {
    this.#prune(now, Math.max(windowMs, throughputSpanMs));
    if (this.#startedAt === null || now - this.#startedAt < windowMs) {
      return null;
    }
    if (now - this.#limitedAt < windowMs) {
      return null;
    }
    const bytes = this.#received
      .filter((received) => now - received.at <= windowMs)
      .reduce((sum, received) => sum + received.bytes, 0);

    return (bytes * 8 * 1_000) / windowMs;
  }

  // throughput は、now までの spanMs のあいだに読み終えた塊の量を、読むのにかかった時間の合計で割った速さ。
  // 先に読む経路 (録画の HLS) で、読んでいない間を数えずに回線の速さを出す。読み終えた塊が無ければ null。
  throughput(now: number, spanMs: number = throughputSpanMs): number | null {
    this.#prune(now, Math.max(spanMs, throughputSpanMs));
    let bytes = 0;
    let ms = 0;
    for (const received of this.#received) {
      if (now - received.at <= spanMs && received.transferMs !== null && received.transferMs > 0) {
        bytes += received.bytes;
        ms += received.transferMs;
      }
    }

    return ms > 0 ? (bytes * 8 * 1_000) / ms : null;
  }
}

// throughputSpanMs は、throughput が見る既定の長さ。kyusu のセグメント (2 秒) の数個分。
export const throughputSpanMs = 10_000;
