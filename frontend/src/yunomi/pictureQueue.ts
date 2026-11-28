// ブラウザ内でデコードする経路 (#1516) の、デコードしたコマの待ち行列。
// 画面の切り替わりごとに、映す PTS (fieldPacer.ts) から、映すコマとフィールドを選ぶ。インタレースのコマは、
// コマの前半に 1 枚目のフィールド、後半に 2 枚目のフィールドを映す (どちらを先に出すかは top_field_first に従う)。
// 番を過ぎたコマは、映さずに破棄し、コマ落ちとして数える。遅れて映すと、そこから先が全部ずれるため。

// QueuedPicture は、待っているコマ。slot は、コマを上げたテクスチャなど、呼ぶ側の持ち物。
export type QueuedPicture<Slot> = {
  // pts は、伸ばした PTS。
  pts: number;
  // duration は、インタレースのコマの長さ (2 枚のフィールドを映す長さ)。省けば、隣り合うコマの PTS の差のうち、いちばん小さいものを使う。
  // 2:3 プルダウンから組み直したコマ (#2276) を混ぜると、時刻の差が 1 コマより短くなることがあるので、呼ぶ側が渡す。
  duration?: number;
  interlaced: boolean;
  topFieldFirst: boolean;
  slot: Slot;
};

// FieldChoice は、映すコマとフィールド。parity は、映すフィールドが画像の上 (0) か下 (1) か。
export type FieldChoice<Slot> = {
  picture: QueuedPicture<Slot>;
  // second は、コマの 2 枚目のフィールドか。
  second: boolean;
  parity: 0 | 1;
};

// PictureQueue は、デコードしたコマを PTS の順に持つ。
export class PictureQueue<Slot> {
  private readonly pictures: QueuedPicture<Slot>[] = [];
  // frameDuration は、隣り合うコマの PTS の差のうち、いちばん小さいもの (コマが欠けた所の差を使わない)。
  frameDuration: number | null = null;
  private lastPts: number | null = null;
  // dropped は、映さずに破棄したコマの数。
  dropped = 0;
  // current は、今映しているコマ。
  private current: QueuedPicture<Slot> | null = null;
  // previous は、current の 1 つ前に映したコマ。インタレ解除が、止まっている画素を見分けるのに使う。
  previous: QueuedPicture<Slot> | null = null;

  // release は、使い終えたコマの slot を返す先。
  constructor(private readonly release: (slot: Slot) => void) {}

  get length(): number {
    return this.pictures.length;
  }

  // newest は、いちばん新しいコマの PTS。なければ null。
  get newest(): number | null {
    return this.pictures.at(-1)?.pts ?? null;
  }

  // push は、デコードしたコマを足す。前より古い時刻のコマは破棄して false を返す (slot は release する)。
  push(picture: QueuedPicture<Slot>): boolean {
    if (this.lastPts !== null && picture.pts <= this.lastPts) {
      this.dropped += 1;
      this.release(picture.slot);

      return false;
    }
    if (this.lastPts !== null) {
      const delta = picture.pts - this.lastPts;
      if (this.frameDuration === null || delta < this.frameDuration) {
        this.frameDuration = delta;
      }
    }
    this.lastPts = picture.pts;
    this.pictures.push(picture);

    return true;
  }

  // choose は、PTS target に映すコマとフィールドを返す。まだ映すものがなければ null。
  choose(target: number): FieldChoice<Slot> | null {
    // 次のコマの番が来ていれば、前のコマは破棄する。
    while (this.pictures.length >= 2 && this.pictures[1]!.pts <= target) {
      const old = this.pictures.shift()!;
      this.retire(old);
    }
    const picture = this.pictures[0];
    if (picture === undefined || picture.pts > target) {
      return null;
    }
    // 映していたコマは、待ち行列から外すとき (retire) に null に戻るので、ここで替わるのは新しいコマだけ。
    this.current = picture;
    const fieldDuration = (picture.duration ?? this.frameDuration ?? 0) / 2;
    const second = picture.interlaced && fieldDuration > 0 && target >= picture.pts + fieldDuration;
    const parity: 0 | 1 = picture.interlaced && second === picture.topFieldFirst ? 1 : 0;

    return { picture, second, parity };
  }

  // following は、今映しているコマの次に映すコマ。choose の後に読む。なければ null。コマの間を補間するのに使う。
  get following(): QueuedPicture<Slot> | null {
    return this.current !== null && this.pictures[0] === this.current
      ? (this.pictures[1] ?? null)
      : null;
  }

  // clear は、待っているコマをすべて破棄する (slot は release する)。選局したときと、飛んだときに呼ぶ。
  // keep の slot は release しない。呼んだ側が持ち続け、使い終えたら自分で返す。
  clear(keep: Slot | null = null): void {
    const release = (slot: Slot) => {
      if (slot !== keep) {
        this.release(slot);
      }
    };
    for (const picture of this.pictures.splice(0)) {
      if (picture !== this.current) {
        release(picture.slot);
      }
    }
    if (this.current !== null) {
      release(this.current.slot);
    }
    if (this.previous !== null && this.previous !== this.current) {
      release(this.previous.slot);
    }
    this.current = null;
    this.previous = null;
    this.lastPts = null;
    this.frameDuration = null;
  }

  // retire は、待ち行列から外したコマを片付ける。映していたコマは、前のコマとして残す。
  private retire(old: QueuedPicture<Slot>): void {
    if (old === this.current) {
      if (this.previous !== null) {
        this.release(this.previous.slot);
      }
      this.previous = old;
      this.current = null;

      return;
    }
    // 一度も映さずに番を過ぎた。
    this.dropped += 1;
    this.release(old.slot);
  }
}
