/**
 * @generated SignedSource<<6732a16d0bcaa79af2ae4223b0883258>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type VideoCodec = "H264" | "HEVC" | "%future added value";
export type VideoFormat = "HD" | "SHV" | "UHD" | "%future added value";
export type playLibraryFileQuery$variables = {
  library: string;
  path: string;
};
export type playLibraryFileQuery$data = {
  readonly libraryFile: {
    readonly hlsUrl: string;
    readonly id: string;
    readonly index: {
      readonly complete: boolean;
      readonly durationSeconds: number | null | undefined;
      readonly seekableSeconds: number;
      readonly writing: boolean;
    };
    readonly library: {
      readonly name: string;
    };
    readonly modifiedAt: string;
    readonly name: string;
    readonly size: number;
    readonly streamUrl: string;
    readonly videoFormat: VideoFormat;
    readonly watchPositionMs: number | null | undefined;
  };
  readonly transcoding: {
    readonly available: boolean;
    readonly presets: ReadonlyArray<{
      readonly codec: VideoCodec;
      readonly name: string;
      readonly passthrough: boolean;
    }>;
  };
};
export type playLibraryFileQuery = {
  response: playLibraryFileQuery$data;
  variables: playLibraryFileQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "library"
  },
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "path"
  }
],
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v2 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "library",
        "variableName": "library"
      },
      {
        "kind": "Variable",
        "name": "path",
        "variableName": "path"
      }
    ],
    "concreteType": "LibraryFile",
    "kind": "LinkedField",
    "name": "libraryFile",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "id",
        "storageKey": null
      },
      (v1/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "size",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "modifiedAt",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "videoFormat",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "watchPositionMs",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "streamUrl",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "hlsUrl",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "Library",
        "kind": "LinkedField",
        "name": "library",
        "plural": false,
        "selections": [
          (v1/*:: as any*/)
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "LibraryFileIndex",
        "kind": "LinkedField",
        "name": "index",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "complete",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "writing",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "seekableSeconds",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "durationSeconds",
            "storageKey": null
          }
        ],
        "storageKey": null
      }
    ],
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "concreteType": "Transcoding",
    "kind": "LinkedField",
    "name": "transcoding",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "available",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "StreamPreset",
        "kind": "LinkedField",
        "name": "presets",
        "plural": true,
        "selections": [
          (v1/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "codec",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "passthrough",
            "storageKey": null
          }
        ],
        "storageKey": null
      }
    ],
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "playLibraryFileQuery",
    "selections": (v2/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "playLibraryFileQuery",
    "selections": (v2/*:: as any*/)
  },
  "params": {
    "cacheID": "40d3ad4c6cbc2b782438b0184ea0f2c6",
    "id": null,
    "metadata": {},
    "name": "playLibraryFileQuery",
    "operationKind": "query",
    "text": "query playLibraryFileQuery(\n  $library: String!\n  $path: String!\n) {\n  libraryFile(library: $library, path: $path) {\n    id\n    name\n    size\n    modifiedAt\n    videoFormat\n    watchPositionMs\n    streamUrl\n    hlsUrl\n    library {\n      name\n    }\n    index {\n      complete\n      writing\n      seekableSeconds\n      durationSeconds\n    }\n  }\n  transcoding {\n    available\n    presets {\n      name\n      codec\n      passthrough\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "ec197491a018870c511dde6c039ef25c";

export default node;
