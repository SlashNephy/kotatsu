/**
 * @generated SignedSource<<5c108817deb7870ba660a159396957f5>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type RecordingFileKind = "OTHER" | "SEGMENT" | "%future added value";
export type StreamKind = "AUDIO" | "OTHER" | "PSI" | "UNKNOWN" | "VIDEO" | "%future added value";
export type VideoCodec = "H264" | "HEVC" | "%future added value";
export type VideoFormat = "HD" | "SHV" | "UHD" | "%future added value";
export type RecordingIdPlayQuery$variables = {
  id: string;
};
export type RecordingIdPlayQuery$data = {
  readonly node: {
    readonly __typename: "Recording";
    readonly audios: ReadonlyArray<{
      readonly componentType: number;
      readonly languages: ReadonlyArray<string>;
      readonly samplingRate: number;
    }>;
    readonly cmSections: ReadonlyArray<{
      readonly from: string;
      readonly to: string;
    }>;
    readonly description: string;
    readonly drops: number;
    readonly endAt: string;
    readonly errors: number;
    readonly files: ReadonlyArray<{
      readonly kind: RecordingFileKind;
      readonly size: number;
    }>;
    readonly gaps: ReadonlyArray<{
      readonly from: string;
      readonly to: string;
    }>;
    readonly jikkyoCommentsUrl: string;
    readonly name: string;
    readonly previewUrl: string | null | undefined;
    readonly recordedFrom: string;
    readonly recordedUntil: string | null | undefined;
    readonly rule: {
      readonly id: string;
      readonly name: string;
    } | null | undefined;
    readonly scrambled: number;
    readonly service: {
      readonly jikkyo: {
        readonly id: string;
        readonly name: string;
      } | null | undefined;
      readonly logoUrl: string | null | undefined;
      readonly name: string;
    };
    readonly startAt: string;
    readonly streamQuality: ReadonlyArray<{
      readonly kind: StreamKind;
      readonly packets: number;
    }>;
    readonly streamUrl: string;
    readonly thumbnailUrl: string | null | undefined;
    readonly videoFormat: VideoFormat;
    readonly watchPositionMs: number | null | undefined;
  } | {
    // This will never be '%other', but we need some
    // value in case none of the concrete values match.
    readonly __typename: "%other";
  } | null | undefined;
  readonly transcoding: {
    readonly available: boolean;
    readonly presets: ReadonlyArray<{
      readonly codec: VideoCodec;
      readonly name: string;
      readonly passthrough: boolean;
    }>;
  };
};
export type RecordingIdPlayQuery = {
  response: RecordingIdPlayQuery$data;
  variables: RecordingIdPlayQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "id"
  }
],
v1 = [
  {
    "kind": "Variable",
    "name": "id",
    "variableName": "id"
  }
],
v2 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "__typename",
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
  "name": "description",
  "storageKey": null
},
v5 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "startAt",
  "storageKey": null
},
v6 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "endAt",
  "storageKey": null
},
v7 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "streamUrl",
  "storageKey": null
},
v8 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "videoFormat",
  "storageKey": null
},
v9 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "recordedFrom",
  "storageKey": null
},
v10 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "recordedUntil",
  "storageKey": null
},
v11 = [
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "from",
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "to",
    "storageKey": null
  }
],
v12 = {
  "alias": null,
  "args": null,
  "concreteType": "RecordingGap",
  "kind": "LinkedField",
  "name": "gaps",
  "plural": true,
  "selections": (v11/*:: as any*/),
  "storageKey": null
},
v13 = {
  "alias": null,
  "args": null,
  "concreteType": "RecordingCMSection",
  "kind": "LinkedField",
  "name": "cmSections",
  "plural": true,
  "selections": (v11/*:: as any*/),
  "storageKey": null
},
v14 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "jikkyoCommentsUrl",
  "storageKey": null
},
v15 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v16 = [
  (v15/*:: as any*/),
  (v3/*:: as any*/)
],
v17 = {
  "alias": null,
  "args": null,
  "concreteType": "Rule",
  "kind": "LinkedField",
  "name": "rule",
  "plural": false,
  "selections": (v16/*:: as any*/),
  "storageKey": null
},
v18 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "watchPositionMs",
  "storageKey": null
},
v19 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "thumbnailUrl",
  "storageKey": null
},
v20 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "previewUrl",
  "storageKey": null
},
v21 = {
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
v22 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "kind",
  "storageKey": null
},
v23 = {
  "alias": null,
  "args": null,
  "concreteType": "RecordingFile",
  "kind": "LinkedField",
  "name": "files",
  "plural": true,
  "selections": [
    (v22/*:: as any*/),
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "size",
      "storageKey": null
    }
  ],
  "storageKey": null
},
v24 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "drops",
  "storageKey": null
},
v25 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "errors",
  "storageKey": null
},
v26 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "scrambled",
  "storageKey": null
},
v27 = {
  "alias": null,
  "args": null,
  "concreteType": "StreamQuality",
  "kind": "LinkedField",
  "name": "streamQuality",
  "plural": true,
  "selections": [
    (v22/*:: as any*/),
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "packets",
      "storageKey": null
    }
  ],
  "storageKey": null
},
v28 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "logoUrl",
  "storageKey": null
},
v29 = {
  "alias": null,
  "args": null,
  "concreteType": "JikkyoChannel",
  "kind": "LinkedField",
  "name": "jikkyo",
  "plural": false,
  "selections": (v16/*:: as any*/),
  "storageKey": null
},
v30 = {
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
    "name": "RecordingIdPlayQuery",
    "selections": [
      {
        "alias": null,
        "args": (v1/*:: as any*/),
        "concreteType": null,
        "kind": "LinkedField",
        "name": "node",
        "plural": false,
        "selections": [
          (v2/*:: as any*/),
          {
            "kind": "InlineFragment",
            "selections": [
              (v3/*:: as any*/),
              (v4/*:: as any*/),
              (v5/*:: as any*/),
              (v6/*:: as any*/),
              (v7/*:: as any*/),
              (v8/*:: as any*/),
              (v9/*:: as any*/),
              (v10/*:: as any*/),
              (v12/*:: as any*/),
              (v13/*:: as any*/),
              (v14/*:: as any*/),
              (v17/*:: as any*/),
              (v18/*:: as any*/),
              (v19/*:: as any*/),
              (v20/*:: as any*/),
              (v21/*:: as any*/),
              (v23/*:: as any*/),
              (v24/*:: as any*/),
              (v25/*:: as any*/),
              (v26/*:: as any*/),
              (v27/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "Service",
                "kind": "LinkedField",
                "name": "service",
                "plural": false,
                "selections": [
                  (v3/*:: as any*/),
                  (v28/*:: as any*/),
                  (v29/*:: as any*/)
                ],
                "storageKey": null
              }
            ],
            "type": "Recording",
            "abstractKey": null
          }
        ],
        "storageKey": null
      },
      (v30/*:: as any*/)
    ],
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "RecordingIdPlayQuery",
    "selections": [
      {
        "alias": null,
        "args": (v1/*:: as any*/),
        "concreteType": null,
        "kind": "LinkedField",
        "name": "node",
        "plural": false,
        "selections": [
          (v2/*:: as any*/),
          {
            "kind": "InlineFragment",
            "selections": [
              (v3/*:: as any*/),
              (v4/*:: as any*/),
              (v5/*:: as any*/),
              (v6/*:: as any*/),
              (v7/*:: as any*/),
              (v8/*:: as any*/),
              (v9/*:: as any*/),
              (v10/*:: as any*/),
              (v12/*:: as any*/),
              (v13/*:: as any*/),
              (v14/*:: as any*/),
              (v17/*:: as any*/),
              (v18/*:: as any*/),
              (v19/*:: as any*/),
              (v20/*:: as any*/),
              (v21/*:: as any*/),
              (v23/*:: as any*/),
              (v24/*:: as any*/),
              (v25/*:: as any*/),
              (v26/*:: as any*/),
              (v27/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "Service",
                "kind": "LinkedField",
                "name": "service",
                "plural": false,
                "selections": [
                  (v3/*:: as any*/),
                  (v28/*:: as any*/),
                  (v29/*:: as any*/),
                  (v15/*:: as any*/)
                ],
                "storageKey": null
              }
            ],
            "type": "Recording",
            "abstractKey": null
          },
          (v15/*:: as any*/)
        ],
        "storageKey": null
      },
      (v30/*:: as any*/)
    ]
  },
  "params": {
    "cacheID": "bc28b6415f282e2d1951a0cdcc758bc0",
    "id": null,
    "metadata": {},
    "name": "RecordingIdPlayQuery",
    "operationKind": "query",
    "text": "query RecordingIdPlayQuery(\n  $id: ID!\n) {\n  node(id: $id) {\n    __typename\n    ... on Recording {\n      name\n      description\n      startAt\n      endAt\n      streamUrl\n      videoFormat\n      recordedFrom\n      recordedUntil\n      gaps {\n        from\n        to\n      }\n      cmSections {\n        from\n        to\n      }\n      jikkyoCommentsUrl\n      rule {\n        id\n        name\n      }\n      watchPositionMs\n      thumbnailUrl\n      previewUrl\n      audios {\n        componentType\n        languages\n        samplingRate\n      }\n      files {\n        kind\n        size\n      }\n      drops\n      errors\n      scrambled\n      streamQuality {\n        kind\n        packets\n      }\n      service {\n        name\n        logoUrl\n        jikkyo {\n          id\n          name\n        }\n        id\n      }\n    }\n    id\n  }\n  transcoding {\n    available\n    presets {\n      name\n      codec\n      passthrough\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "0bfbd7dcdbfdfb2348f351d9a1fbf9b9";

export default node;
