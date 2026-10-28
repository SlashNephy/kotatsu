/**
 * @generated SignedSource<<099d709d10931d1e01654ec123386069>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
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
    }>;
    readonly endAt: string;
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
    readonly service: {
      readonly jikkyo: {
        readonly id: string;
        readonly name: string;
      } | null | undefined;
      readonly logoUrl: string | null | undefined;
      readonly name: string;
    };
    readonly startAt: string;
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
  "name": "startAt",
  "storageKey": null
},
v5 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "endAt",
  "storageKey": null
},
v6 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "streamUrl",
  "storageKey": null
},
v7 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "videoFormat",
  "storageKey": null
},
v8 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "recordedFrom",
  "storageKey": null
},
v9 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "recordedUntil",
  "storageKey": null
},
v10 = {
  "alias": null,
  "args": null,
  "concreteType": "RecordingGap",
  "kind": "LinkedField",
  "name": "gaps",
  "plural": true,
  "selections": [
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
  "name": "id",
  "storageKey": null
},
v13 = [
  (v12/*:: as any*/),
  (v3/*:: as any*/)
],
v14 = {
  "alias": null,
  "args": null,
  "concreteType": "Rule",
  "kind": "LinkedField",
  "name": "rule",
  "plural": false,
  "selections": (v13/*:: as any*/),
  "storageKey": null
},
v15 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "watchPositionMs",
  "storageKey": null
},
v16 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "thumbnailUrl",
  "storageKey": null
},
v17 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "previewUrl",
  "storageKey": null
},
v18 = {
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
    }
  ],
  "storageKey": null
},
v19 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "logoUrl",
  "storageKey": null
},
v20 = {
  "alias": null,
  "args": null,
  "concreteType": "JikkyoChannel",
  "kind": "LinkedField",
  "name": "jikkyo",
  "plural": false,
  "selections": (v13/*:: as any*/),
  "storageKey": null
},
v21 = {
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
              (v11/*:: as any*/),
              (v14/*:: as any*/),
              (v15/*:: as any*/),
              (v16/*:: as any*/),
              (v17/*:: as any*/),
              (v18/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "Service",
                "kind": "LinkedField",
                "name": "service",
                "plural": false,
                "selections": [
                  (v3/*:: as any*/),
                  (v19/*:: as any*/),
                  (v20/*:: as any*/)
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
      (v21/*:: as any*/)
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
              (v11/*:: as any*/),
              (v14/*:: as any*/),
              (v15/*:: as any*/),
              (v16/*:: as any*/),
              (v17/*:: as any*/),
              (v18/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "Service",
                "kind": "LinkedField",
                "name": "service",
                "plural": false,
                "selections": [
                  (v3/*:: as any*/),
                  (v19/*:: as any*/),
                  (v20/*:: as any*/),
                  (v12/*:: as any*/)
                ],
                "storageKey": null
              }
            ],
            "type": "Recording",
            "abstractKey": null
          },
          (v12/*:: as any*/)
        ],
        "storageKey": null
      },
      (v21/*:: as any*/)
    ]
  },
  "params": {
    "cacheID": "17b0fdd381426f3d9ad7d390652614c8",
    "id": null,
    "metadata": {},
    "name": "RecordingIdPlayQuery",
    "operationKind": "query",
    "text": "query RecordingIdPlayQuery(\n  $id: ID!\n) {\n  node(id: $id) {\n    __typename\n    ... on Recording {\n      name\n      startAt\n      endAt\n      streamUrl\n      videoFormat\n      recordedFrom\n      recordedUntil\n      gaps {\n        from\n        to\n      }\n      jikkyoCommentsUrl\n      rule {\n        id\n        name\n      }\n      watchPositionMs\n      thumbnailUrl\n      previewUrl\n      audios {\n        componentType\n        languages\n      }\n      service {\n        name\n        logoUrl\n        jikkyo {\n          id\n          name\n        }\n        id\n      }\n    }\n    id\n  }\n  transcoding {\n    available\n    presets {\n      name\n      codec\n      passthrough\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "eeb6e04fca6418b638890daa3d8f25b2";

export default node;
