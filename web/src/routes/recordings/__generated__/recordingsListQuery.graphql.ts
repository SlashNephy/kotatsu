/**
 * @generated SignedSource<<074ed1948645a851014f842cedc1619f>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type RecordingState = "COMPLETED" | "COMPLETED_WITH_ERRORS" | "FAILED" | "PARTIAL" | "RECORDING" | "%future added value";
export type RecordingFilter = {
  keyword?: string | null | undefined;
  serviceIds?: ReadonlyArray<string> | null | undefined;
  since?: string | null | undefined;
  states?: ReadonlyArray<RecordingState> | null | undefined;
  tagIds?: ReadonlyArray<string> | null | undefined;
  until?: string | null | undefined;
};
export type recordingsListQuery$variables = {
  after?: string | null | undefined;
  filter?: RecordingFilter | null | undefined;
  first: number;
};
export type recordingsListQuery$data = {
  readonly recordings: {
    readonly edges: ReadonlyArray<{
      readonly node: {
        readonly endAt: string;
        readonly id: string;
        readonly name: string;
        readonly service: {
          readonly logoUrl: string | null | undefined;
          readonly name: string;
        };
        readonly startAt: string;
        readonly state: RecordingState;
        readonly tags: ReadonlyArray<{
          readonly id: string;
          readonly name: string;
        }>;
        readonly thumbnailUrl: string | null | undefined;
      };
    }>;
    readonly pageInfo: {
      readonly endCursor: string | null | undefined;
      readonly hasNextPage: boolean;
    };
  };
};
export type recordingsListQuery = {
  response: recordingsListQuery$data;
  variables: recordingsListQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "after"
},
v1 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "filter"
},
v2 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "first"
},
v3 = [
  {
    "kind": "Variable",
    "name": "after",
    "variableName": "after"
  },
  {
    "kind": "Variable",
    "name": "filter",
    "variableName": "filter"
  },
  {
    "kind": "Variable",
    "name": "first",
    "variableName": "first"
  }
],
v4 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v5 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v6 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "startAt",
  "storageKey": null
},
v7 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "endAt",
  "storageKey": null
},
v8 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "state",
  "storageKey": null
},
v9 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "thumbnailUrl",
  "storageKey": null
},
v10 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "logoUrl",
  "storageKey": null
},
v11 = {
  "alias": null,
  "args": null,
  "concreteType": "Tag",
  "kind": "LinkedField",
  "name": "tags",
  "plural": true,
  "selections": [
    (v4/*:: as any*/),
    (v5/*:: as any*/)
  ],
  "storageKey": null
},
v12 = {
  "alias": null,
  "args": null,
  "concreteType": "PageInfo",
  "kind": "LinkedField",
  "name": "pageInfo",
  "plural": false,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "hasNextPage",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "endCursor",
      "storageKey": null
    }
  ],
  "storageKey": null
};
return {
  "fragment": {
    "argumentDefinitions": [
      (v0/*:: as any*/),
      (v1/*:: as any*/),
      (v2/*:: as any*/)
    ],
    "kind": "Fragment",
    "metadata": null,
    "name": "recordingsListQuery",
    "selections": [
      {
        "alias": null,
        "args": (v3/*:: as any*/),
        "concreteType": "RecordingConnection",
        "kind": "LinkedField",
        "name": "recordings",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "concreteType": "RecordingEdge",
            "kind": "LinkedField",
            "name": "edges",
            "plural": true,
            "selections": [
              {
                "alias": null,
                "args": null,
                "concreteType": "Recording",
                "kind": "LinkedField",
                "name": "node",
                "plural": false,
                "selections": [
                  (v4/*:: as any*/),
                  (v5/*:: as any*/),
                  (v6/*:: as any*/),
                  (v7/*:: as any*/),
                  (v8/*:: as any*/),
                  (v9/*:: as any*/),
                  {
                    "alias": null,
                    "args": null,
                    "concreteType": "Service",
                    "kind": "LinkedField",
                    "name": "service",
                    "plural": false,
                    "selections": [
                      (v5/*:: as any*/),
                      (v10/*:: as any*/)
                    ],
                    "storageKey": null
                  },
                  (v11/*:: as any*/)
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          },
          (v12/*:: as any*/)
        ],
        "storageKey": null
      }
    ],
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [
      (v2/*:: as any*/),
      (v0/*:: as any*/),
      (v1/*:: as any*/)
    ],
    "kind": "Operation",
    "name": "recordingsListQuery",
    "selections": [
      {
        "alias": null,
        "args": (v3/*:: as any*/),
        "concreteType": "RecordingConnection",
        "kind": "LinkedField",
        "name": "recordings",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "concreteType": "RecordingEdge",
            "kind": "LinkedField",
            "name": "edges",
            "plural": true,
            "selections": [
              {
                "alias": null,
                "args": null,
                "concreteType": "Recording",
                "kind": "LinkedField",
                "name": "node",
                "plural": false,
                "selections": [
                  (v4/*:: as any*/),
                  (v5/*:: as any*/),
                  (v6/*:: as any*/),
                  (v7/*:: as any*/),
                  (v8/*:: as any*/),
                  (v9/*:: as any*/),
                  {
                    "alias": null,
                    "args": null,
                    "concreteType": "Service",
                    "kind": "LinkedField",
                    "name": "service",
                    "plural": false,
                    "selections": [
                      (v5/*:: as any*/),
                      (v10/*:: as any*/),
                      (v4/*:: as any*/)
                    ],
                    "storageKey": null
                  },
                  (v11/*:: as any*/)
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          },
          (v12/*:: as any*/)
        ],
        "storageKey": null
      }
    ]
  },
  "params": {
    "cacheID": "999d1b24bbb00685955523f6a2a84a6f",
    "id": null,
    "metadata": {},
    "name": "recordingsListQuery",
    "operationKind": "query",
    "text": "query recordingsListQuery(\n  $first: Int!\n  $after: String\n  $filter: RecordingFilter\n) {\n  recordings(first: $first, after: $after, filter: $filter) {\n    edges {\n      node {\n        id\n        name\n        startAt\n        endAt\n        state\n        thumbnailUrl\n        service {\n          name\n          logoUrl\n          id\n        }\n        tags {\n          id\n          name\n        }\n      }\n    }\n    pageInfo {\n      hasNextPage\n      endCursor\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "99fe0ff4f85250d7b37c875f5b0302c4";

export default node;
