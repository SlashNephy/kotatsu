/**
 * @generated SignedSource<<0e13d2b5dae01cf47619738f5594a367>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type RecordingFailureReason = "CANCELLED" | "PROGRAM_NOT_FOUND" | "STORAGE_FULL" | "STORAGE_UNAVAILABLE" | "STREAM_INTERRUPTED" | "TUNER_SERVER_UNREACHABLE" | "TUNER_UNAVAILABLE" | "%future added value";
export type RecordingFileKind = "INDEX" | "OTHER" | "PREVIEW" | "SEGMENT" | "THUMBNAIL" | "%future added value";
export type RecordingState = "COMPLETED" | "COMPLETED_WITH_ERRORS" | "FAILED" | "PARTIAL" | "RECORDING" | "%future added value";
export type Role = "ADMIN" | "MEMBER" | "VIEWER" | "%future added value";
export type RecordingIdQuery$variables = {
  id: string;
};
export type RecordingIdQuery$data = {
  readonly node: {
    readonly __typename: "Recording";
    readonly description: string;
    readonly drops: any;
    readonly endAt: string;
    readonly errors: any;
    readonly failureReason: RecordingFailureReason | null | undefined;
    readonly files: ReadonlyArray<{
      readonly kind: RecordingFileKind;
      readonly size: any;
    }>;
    readonly gaps: ReadonlyArray<{
      readonly from: string;
      readonly to: string;
    }>;
    readonly id: string;
    readonly name: string;
    readonly programLost: boolean;
    readonly protected: boolean;
    readonly recordedFrom: string;
    readonly recordedUntil: string | null | undefined;
    readonly scrambled: any;
    readonly service: {
      readonly name: string;
    };
    readonly startAt: string;
    readonly state: RecordingState;
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
  (v2/*:: as any*/)
],
v4 = [
  {
    "kind": "Variable",
    "name": "id",
    "variableName": "id"
  }
],
v5 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "__typename",
  "storageKey": null
},
v6 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v7 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "description",
  "storageKey": null
},
v8 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "startAt",
  "storageKey": null
},
v9 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "endAt",
  "storageKey": null
},
v10 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "state",
  "storageKey": null
},
v11 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "failureReason",
  "storageKey": null
},
v12 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "protected",
  "storageKey": null
},
v13 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "drops",
  "storageKey": null
},
v14 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "errors",
  "storageKey": null
},
v15 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "scrambled",
  "storageKey": null
},
v16 = {
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
v17 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "programLost",
  "storageKey": null
},
v18 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "thumbnailUrl",
  "storageKey": null
},
v19 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "recordedFrom",
  "storageKey": null
},
v20 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "recordedUntil",
  "storageKey": null
},
v21 = {
  "alias": null,
  "args": null,
  "concreteType": "Tag",
  "kind": "LinkedField",
  "name": "tags",
  "plural": true,
  "selections": [
    (v6/*:: as any*/),
    (v2/*:: as any*/)
  ],
  "storageKey": null
},
v22 = {
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
      "name": "size",
      "storageKey": null
    }
  ],
  "storageKey": null
},
v23 = [
  (v2/*:: as any*/),
  (v6/*:: as any*/)
];
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
        "selections": (v3/*:: as any*/),
        "storageKey": null
      },
      {
        "alias": null,
        "args": (v4/*:: as any*/),
        "concreteType": null,
        "kind": "LinkedField",
        "name": "node",
        "plural": false,
        "selections": [
          (v5/*:: as any*/),
          {
            "kind": "InlineFragment",
            "selections": [
              (v6/*:: as any*/),
              (v2/*:: as any*/),
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
              {
                "alias": null,
                "args": null,
                "concreteType": "Service",
                "kind": "LinkedField",
                "name": "service",
                "plural": false,
                "selections": (v3/*:: as any*/),
                "storageKey": null
              },
              (v21/*:: as any*/),
              (v22/*:: as any*/)
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
          (v6/*:: as any*/)
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
        "selections": (v23/*:: as any*/),
        "storageKey": null
      },
      {
        "alias": null,
        "args": (v4/*:: as any*/),
        "concreteType": null,
        "kind": "LinkedField",
        "name": "node",
        "plural": false,
        "selections": [
          (v5/*:: as any*/),
          (v6/*:: as any*/),
          {
            "kind": "InlineFragment",
            "selections": [
              (v2/*:: as any*/),
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
              {
                "alias": null,
                "args": null,
                "concreteType": "Service",
                "kind": "LinkedField",
                "name": "service",
                "plural": false,
                "selections": (v23/*:: as any*/),
                "storageKey": null
              },
              (v21/*:: as any*/),
              (v22/*:: as any*/)
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
    "cacheID": "8f1767794dda8d3d02344b19866f5b0b",
    "id": null,
    "metadata": {},
    "name": "RecordingIdQuery",
    "operationKind": "query",
    "text": "query RecordingIdQuery(\n  $id: ID!\n) {\n  viewer {\n    role\n    id\n  }\n  tags {\n    name\n    id\n  }\n  node(id: $id) {\n    __typename\n    ... on Recording {\n      id\n      name\n      description\n      startAt\n      endAt\n      state\n      failureReason\n      protected\n      drops\n      errors\n      scrambled\n      gaps {\n        from\n        to\n      }\n      programLost\n      thumbnailUrl\n      recordedFrom\n      recordedUntil\n      service {\n        name\n        id\n      }\n      tags {\n        id\n        name\n      }\n      files {\n        kind\n        size\n      }\n    }\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "9b765b8529bee96ffdce2f38a8277b7d";

export default node;
