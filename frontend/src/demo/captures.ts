// デモのキャプチャ (#908)。tenban の代わりに、撮ったキャプチャをメモリ上に残す。画面を読み直すと消える。

type Metadata = {
  serviceName: string;
  programName: string | null;
  caption: string | null;
  recordingId: string | null;
  position: number | null;
  broadcastAt: string;
  takenAt: string;
};

type DemoCapture = Metadata & { id: string; imageUrl: string; size: number };

export type DemoCaptures = {
  // add は、POST /media/captures の本文を受け取って残し、残したキャプチャの ID を返す。
  add: (body: FormData) => Promise<string>;
  list: (args: { keyword?: string | null }) => object;
  remove: (id: string) => string;
};

function normalize(text: string): string {
  return text.normalize('NFKC').toLowerCase();
}

export function createDemoCaptures(): DemoCaptures {
  const captures: DemoCapture[] = [];
  let next = 1;

  return {
    add: async (body) => {
      const metadata = body.get('metadata');
      const image = body.get('image');
      if (!(metadata instanceof Blob) || !(image instanceof Blob)) {
        throw new Error('the capture body is incomplete');
      }
      const id = `Capture:demo-${next}`;
      next += 1;
      captures.unshift({
        ...(JSON.parse(await metadata.text()) as Metadata),
        id,
        imageUrl: URL.createObjectURL(image),
        size: image.size,
      });

      return id;
    },
    list: ({ keyword }) => {
      const words = normalize(keyword ?? '')
        .split(/\s+/)
        .filter((word) => word !== '');
      const matched = captures.filter((capture) => {
        const haystack = normalize(`${capture.programName ?? ''} ${capture.serviceName}`);

        return words.every((word) => haystack.includes(word));
      });

      return {
        edges: matched.map((node) => ({ cursor: node.id, node })),
        pageInfo: {
          hasNextPage: false,
          hasPreviousPage: false,
          startCursor: null,
          endCursor: null,
        },
      };
    },
    remove: (id) => {
      const index = captures.findIndex((capture) => capture.id === id);
      if (index < 0) {
        throw new Error('capture not found');
      }
      const [removed] = captures.splice(index, 1);
      if (removed !== undefined) {
        URL.revokeObjectURL(removed.imageUrl);
      }

      return id;
    },
  };
}
