/**
 * @generated SignedSource<<7e5ba4df7cbc24dfe9dbaac9f584fff1>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type RecordingFailureReason = "CANCELLED" | "PROGRAM_NOT_FOUND" | "RECORDER_UNAVAILABLE" | "STORAGE_FULL" | "STORAGE_UNAVAILABLE" | "STREAM_INTERRUPTED" | "TUNER_SERVER_UNREACHABLE" | "TUNER_UNAVAILABLE" | "%future added value";
export type RecordingFileKind = "INDEX" | "OTHER" | "PREVIEW" | "SEGMENT" | "THUMBNAIL" | "%future added value";
export type RecordingState = "COMPLETED" | "COMPLETED_WITH_ERRORS" | "FAILED" | "PARTIAL" | "RECORDING" | "%future added value";
export type Role = "ADMIN" | "MEMBER" | "VIEWER" | "%future added value";
export type RecordingIdQuery$variables = {
  id: string;
};
export type RecordingIdQuery$data = {
  readonly node: {
    readonly __typename: "Recording";
    readonly broadcastEndedAt: string | null | undefined;
    readonly broadcastStartedAt: string | null | undefined;
    readonly description: string;
    readonly drops: number;
    readonly endAt: string;
    readonly errors: number;
    readonly failureReason: RecordingFailureReason | null | undefined;
    readonly files: ReadonlyArray<{
      readonly kind: RecordingFileKind;
      readonly path: string;
      readonly size: number;
      readonly url: string | null | undefined;
    }>;
    readonly gaps: ReadonlyArray<{
      readonly from: string;
      readonly to: string;
    }>;
    readonly genres: ReadonlyArray<{
      readonly lv1: number;
    }>;
    readonly id: string;
    readonly name: string;
    readonly notBroadcast: boolean;
    readonly programLost: boolean;
    readonly protected: boolean;
    readonly recordedFrom: string;
    readonly recordedUntil: string | null | undefined;
    readonly rule: {
      readonly id: string;
    } | null | undefined;
    readonly ruleName: string | null | undefined;
    readonly scrambled: number;
    readonly service: {
      readonly logoUrl: string | null | undefined;
      readonly name: string;
    };
    readonly startAt: string;
    readonly state: RecordingState;
    readonly streamUrl: string;
    readonly tags: ReadonlyArray<{
      readonly id: string;
      readonly name: string;
    }>;
    readonly thumbnailUrl: string | null | undefined;
  } | {
    // This will never be '%other', but we need some
    // value in case none of the concrete values match.
    readonly __typename: "%other";
  } | null | undefined;
  readonly tags: ReadonlyArray<{
    readonly name: string;
  }>;
  readonly viewer: {
    readonly role: Role;
  };
};
export type RecordingIdQuery = {
  response: RecordingIdQuery$data;
  variables: RecordingIdQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "id"
  }
],
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "role",
  "storageKey": null
},
v2 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v3 = [
  {
    "kind": "Variable",
    "name": "id",
    "variableName": "id"
  }
],
v4 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "__typename",
  "storageKey": null
},
v5 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v6 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "description",
  "storageKey": null
},
v7 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "startAt",
  "storageKey": null
},
v8 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "endAt",
  "storageKey": null
},
v9 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "state",
  "storageKey": null
},
v10 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "failureReason",
  "storageKey": null
},
v11 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "protected",
  "storageKey": null
},
v12 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "drops",
  "storageKey": null
},
v13 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "errors",
  "storageKey": null
},
v14 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "scrambled",
  "storageKey": null
},
v15 = {
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
v16 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "programLost",
  "storageKey": null
},
v17 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "broadcastStartedAt",
  "storageKey": null
},
v18 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "broadcastEndedAt",
  "storageKey": null
},
v19 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "notBroadcast",
  "storageKey": null
},
v20 = {
  "alias": null,
  "args": null,
  "concreteType": "Genre",
  "kind": "LinkedField",
  "name": "genres",
  "plural": true,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "lv1",
      "storageKey": null
    }
  ],
  "storageKey": null
},
v21 = {
  "alias": null,
  "args": null,
  "concreteType": "Rule",
  "kind": "LinkedField",
  "name": "rule",
  "plural": false,
  "selections": [
    (v5/*:: as any*/)
  ],
  "storageKey": null
},
v22 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "ruleName",
  "storageKey": null
},
v23 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "thumbnailUrl",
  "storageKey": null
},
v24 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "streamUrl",
  "storageKey": null
},
v25 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "recordedFrom",
  "storageKey": null
},
v26 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "recordedUntil",
  "storageKey": null
},
v27 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "logoUrl",
  "storageKey": null
},
v28 = {
  "alias": null,
  "args": null,
  "concreteType": "Tag",
  "kind": "LinkedField",
  "name": "tags",
  "plural": true,
  "selections": [
    (v5/*:: as any*/),
    (v2/*:: as any*/)
  ],
  "storageKey": null
},
v29 = {
  "alias": null,
  "args": null,
  "concreteType": "RecordingFile",
  "kind": "LinkedField",
  "name": "files",
  "plural": true,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "kind",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "path",
      "storageKey": null
    },
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
      "name": "url",
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
    "name": "RecordingIdQuery",
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "User",
        "kind": "LinkedField",
        "name": "viewer",
        "plural": false,
        "selections": [
          (v1/*:: as any*/)
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "Tag",
        "kind": "LinkedField",
        "name": "tags",
        "plural": true,
        "selections": [
          (v2/*:: as any*/)
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": (v3/*:: as any*/),
        "concreteType": null,
        "kind": "LinkedField",
        "name": "node",
        "plural": false,
        "selections": [
          (v4/*:: as any*/),
          {
            "kind": "InlineFragment",
            "selections": [
              (v5/*:: as any*/),
              (v2/*:: as any*/),
              (v6/*:: as any*/),
              (v7/*:: as any*/),
              (v8/*:: as any*/),
              (v9/*:: as any*/),
              (v10/*:: as any*/),
              (v11/*:: as any*/),
              (v12/*:: as any*/),
              (v13/*:: as any*/),
              (v14/*:: as any*/),
              (v15/*:: as any*/),
              (v16/*:: as any*/),
              (v17/*:: as any*/),
              (v18/*:: as any*/),
              (v19/*:: as any*/),
              (v20/*:: as any*/),
              (v21/*:: as any*/),
              (v22/*:: as any*/),
              (v23/*:: as any*/),
              (v24/*:: as any*/),
              (v25/*:: as any*/),
              (v26/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "Service",
                "kind": "LinkedField",
                "name": "service",
                "plural": false,
                "selections": [
                  (v2/*:: as any*/),
                  (v27/*:: as any*/)
                ],
                "storageKey": null
              },
              (v28/*:: as any*/),
              (v29/*:: as any*/)
            ],
            "type": "Recording",
            "abstractKey": null
          }
        ],
        "storageKey": null
      }
    ],
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "RecordingIdQuery",
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "User",
        "kind": "LinkedField",
        "name": "viewer",
        "plural": false,
        "selections": [
          (v1/*:: as any*/),
          (v5/*:: as any*/)
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "Tag",
        "kind": "LinkedField",
        "name": "tags",
        "plural": true,
        "selections": [
          (v2/*:: as any*/),
          (v5/*:: as any*/)
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": (v3/*:: as any*/),
        "concreteType": null,
        "kind": "LinkedField",
        "name": "node",
        "plural": false,
        "selections": [
          (v4/*:: as any*/),
          (v5/*:: as any*/),
          {
            "kind": "InlineFragment",
            "selections": [
              (v2/*:: as any*/),
              (v6/*:: as any*/),
              (v7/*:: as any*/),
              (v8/*:: as any*/),
              (v9/*:: as any*/),
              (v10/*:: as any*/),
              (v11/*:: as any*/),
              (v12/*:: as any*/),
              (v13/*:: as any*/),
              (v14/*:: as any*/),
              (v15/*:: as any*/),
              (v16/*:: as any*/),
              (v17/*:: as any*/),
              (v18/*:: as any*/),
              (v19/*:: as any*/),
              (v20/*:: as any*/),
              (v21/*:: as any*/),
              (v22/*:: as any*/),
              (v23/*:: as any*/),
              (v24/*:: as any*/),
              (v25/*:: as any*/),
              (v26/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "Service",
                "kind": "LinkedField",
                "name": "service",
                "plural": false,
                "selections": [
                  (v2/*:: as any*/),
                  (v27/*:: as any*/),
                  (v5/*:: as any*/)
                ],
                "storageKey": null
              },
              (v28/*:: as any*/),
              (v29/*:: as any*/)
            ],
            "type": "Recording",
            "abstractKey": null
          }
        ],
        "storageKey": null
      }
    ]
  },
  "params": {
    "cacheID": "0b0c03f2830a273ab570918187354b25",
    "id": null,
    "metadata": {},
    "name": "RecordingIdQuery",
    "operationKind": "query",
    "text": "query RecordingIdQuery(\n  $id: ID!\n) {\n  viewer {\n    role\n    id\n  }\n  tags {\n    name\n    id\n  }\n  node(id: $id) {\n    __typename\n    ... on Recording {\n      id\n      name\n      description\n      startAt\n      endAt\n      state\n      failureReason\n      protected\n      drops\n      errors\n      scrambled\n      gaps {\n        from\n        to\n      }\n      programLost\n      broadcastStartedAt\n      broadcastEndedAt\n      notBroadcast\n      genres {\n        lv1\n      }\n      rule {\n        id\n      }\n      ruleName\n      thumbnailUrl\n      streamUrl\n      recordedFrom\n      recordedUntil\n      service {\n        name\n        logoUrl\n        id\n      }\n      tags {\n        id\n        name\n      }\n      files {\n        kind\n        path\n        size\n        url\n      }\n    }\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "f6b8a1d4cf442874aa3c77615af05e36";

export default node;
