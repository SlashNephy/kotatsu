/**
 * @generated SignedSource<<5984810fcce98c7a49cf624d0b03548f>>
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
    readonly audios: ReadonlyArray<{
      readonly componentType: number;
      readonly languages: ReadonlyArray<string>;
      readonly samplingRate: number;
    }>;
    readonly broadcast: {
      readonly jikkyoChannelId: string | null | undefined;
      readonly program: {
        readonly description: string;
        readonly endAt: string | null | undefined;
        readonly extended: ReadonlyArray<{
          readonly heading: string;
          readonly text: string;
        }>;
        readonly name: string;
        readonly startAt: string | null | undefined;
      } | null | undefined;
      readonly recordedFrom: string | null | undefined;
      readonly service: {
        readonly logoUrl: string | null | undefined;
        readonly name: string;
      } | null | undefined;
      readonly serviceName: string;
    } | null | undefined;
    readonly hlsUrl: string;
    readonly id: string;
    readonly index: {
      readonly complete: boolean;
      readonly durationSeconds: number | null | undefined;
      readonly seekableSeconds: number;
      readonly writing: boolean;
    };
    readonly jikkyoCommentsUrl: string;
    readonly library: {
      readonly name: string;
    };
    readonly modifiedAt: string;
    readonly name: string;
    readonly quality: {
      readonly drops: number;
      readonly errors: number;
      readonly scrambled: number;
    } | null | undefined;
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
v1 = [
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
v2 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v3 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v4 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "size",
  "storageKey": null
},
v5 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "modifiedAt",
  "storageKey": null
},
v6 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "videoFormat",
  "storageKey": null
},
v7 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "watchPositionMs",
  "storageKey": null
},
v8 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "streamUrl",
  "storageKey": null
},
v9 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "hlsUrl",
  "storageKey": null
},
v10 = {
  "alias": null,
  "args": null,
  "concreteType": "Library",
  "kind": "LinkedField",
  "name": "library",
  "plural": false,
  "selections": [
    (v3/*:: as any*/)
  ],
  "storageKey": null
},
v11 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "jikkyoCommentsUrl",
  "storageKey": null
},
v12 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "serviceName",
  "storageKey": null
},
v13 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "jikkyoChannelId",
  "storageKey": null
},
v14 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "recordedFrom",
  "storageKey": null
},
v15 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "logoUrl",
  "storageKey": null
},
v16 = {
  "alias": null,
  "args": null,
  "concreteType": "LibraryProgram",
  "kind": "LinkedField",
  "name": "program",
  "plural": false,
  "selections": [
    (v3/*:: as any*/),
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "description",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "concreteType": "ExtendedItem",
      "kind": "LinkedField",
      "name": "extended",
      "plural": true,
      "selections": [
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "heading",
          "storageKey": null
        },
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "text",
          "storageKey": null
        }
      ],
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "startAt",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "endAt",
      "storageKey": null
    }
  ],
  "storageKey": null
},
v17 = {
  "alias": null,
  "args": null,
  "concreteType": "ProgramAudio",
  "kind": "LinkedField",
  "name": "audios",
  "plural": true,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "componentType",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "languages",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "samplingRate",
      "storageKey": null
    }
  ],
  "storageKey": null
},
v18 = {
  "alias": null,
  "args": null,
  "concreteType": "LibraryFileQuality",
  "kind": "LinkedField",
  "name": "quality",
  "plural": false,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "drops",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "errors",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "scrambled",
      "storageKey": null
    }
  ],
  "storageKey": null
},
v19 = {
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
},
v20 = {
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
        (v3/*:: as any*/),
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
};
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "playLibraryFileQuery",
    "selections": [
      {
        "alias": null,
        "args": (v1/*:: as any*/),
        "concreteType": "LibraryFile",
        "kind": "LinkedField",
        "name": "libraryFile",
        "plural": false,
        "selections": [
          (v2/*:: as any*/),
          (v3/*:: as any*/),
          (v4/*:: as any*/),
          (v5/*:: as any*/),
          (v6/*:: as any*/),
          (v7/*:: as any*/),
          (v8/*:: as any*/),
          (v9/*:: as any*/),
          (v10/*:: as any*/),
          (v11/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "concreteType": "LibraryBroadcast",
            "kind": "LinkedField",
            "name": "broadcast",
            "plural": false,
            "selections": [
              (v12/*:: as any*/),
              (v13/*:: as any*/),
              (v14/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "Service",
                "kind": "LinkedField",
                "name": "service",
                "plural": false,
                "selections": [
                  (v3/*:: as any*/),
                  (v15/*:: as any*/)
                ],
                "storageKey": null
              },
              (v16/*:: as any*/)
            ],
            "storageKey": null
          },
          (v17/*:: as any*/),
          (v18/*:: as any*/),
          (v19/*:: as any*/)
        ],
        "storageKey": null
      },
      (v20/*:: as any*/)
    ],
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "playLibraryFileQuery",
    "selections": [
      {
        "alias": null,
        "args": (v1/*:: as any*/),
        "concreteType": "LibraryFile",
        "kind": "LinkedField",
        "name": "libraryFile",
        "plural": false,
        "selections": [
          (v2/*:: as any*/),
          (v3/*:: as any*/),
          (v4/*:: as any*/),
          (v5/*:: as any*/),
          (v6/*:: as any*/),
          (v7/*:: as any*/),
          (v8/*:: as any*/),
          (v9/*:: as any*/),
          (v10/*:: as any*/),
          (v11/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "concreteType": "LibraryBroadcast",
            "kind": "LinkedField",
            "name": "broadcast",
            "plural": false,
            "selections": [
              (v12/*:: as any*/),
              (v13/*:: as any*/),
              (v14/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "Service",
                "kind": "LinkedField",
                "name": "service",
                "plural": false,
                "selections": [
                  (v3/*:: as any*/),
                  (v15/*:: as any*/),
                  (v2/*:: as any*/)
                ],
                "storageKey": null
              },
              (v16/*:: as any*/)
            ],
            "storageKey": null
          },
          (v17/*:: as any*/),
          (v18/*:: as any*/),
          (v19/*:: as any*/)
        ],
        "storageKey": null
      },
      (v20/*:: as any*/)
    ]
  },
  "params": {
    "cacheID": "7fdbf4aec8146fced346391e93db6834",
    "id": null,
    "metadata": {},
    "name": "playLibraryFileQuery",
    "operationKind": "query",
    "text": "query playLibraryFileQuery(\n  $library: String!\n  $path: String!\n) {\n  libraryFile(library: $library, path: $path) {\n    id\n    name\n    size\n    modifiedAt\n    videoFormat\n    watchPositionMs\n    streamUrl\n    hlsUrl\n    library {\n      name\n    }\n    jikkyoCommentsUrl\n    broadcast {\n      serviceName\n      jikkyoChannelId\n      recordedFrom\n      service {\n        name\n        logoUrl\n        id\n      }\n      program {\n        name\n        description\n        extended {\n          heading\n          text\n        }\n        startAt\n        endAt\n      }\n    }\n    audios {\n      componentType\n      languages\n      samplingRate\n    }\n    quality {\n      drops\n      errors\n      scrambled\n    }\n    index {\n      complete\n      writing\n      seekableSeconds\n      durationSeconds\n    }\n  }\n  transcoding {\n    available\n    presets {\n      name\n      codec\n      passthrough\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "2b7eadf6ac6c0c6d6e664df95b50b2bb";

export default node;
