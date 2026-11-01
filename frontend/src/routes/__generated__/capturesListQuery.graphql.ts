/**
 * @generated SignedSource<<3ef753cc07a5c7f8b4fdd8d28fe7d8a1>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type capturesListQuery$variables = {
  after?: string | null | undefined;
  first: number;
  keyword?: string | null | undefined;
};
export type capturesListQuery$data = {
  readonly captures: {
    readonly edges: ReadonlyArray<{
      readonly node: {
        readonly broadcastAt: string;
        readonly id: string;
        readonly imageUrl: string;
        readonly position: number | null | undefined;
        readonly programName: string | null | undefined;
        readonly recordingId: string | null | undefined;
        readonly serviceName: string;
      };
    }>;
    readonly pageInfo: {
      readonly endCursor: string | null | undefined;
      readonly hasNextPage: boolean;
    };
  };
  readonly capturesAvailable: boolean;
};
export type capturesListQuery = {
  response: capturesListQuery$data;
  variables: capturesListQuery$variables;
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
  "name": "first"
},
v2 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "keyword"
},
v3 = [
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "capturesAvailable",
    "storageKey": null
  },
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "after",
        "variableName": "after"
      },
      {
        "kind": "Variable",
        "name": "first",
        "variableName": "first"
      },
      {
        "kind": "Variable",
        "name": "keyword",
        "variableName": "keyword"
      }
    ],
    "concreteType": "CaptureConnection",
    "kind": "LinkedField",
    "name": "captures",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "CaptureEdge",
        "kind": "LinkedField",
        "name": "edges",
        "plural": true,
        "selections": [
          {
            "alias": null,
            "args": null,
            "concreteType": "Capture",
            "kind": "LinkedField",
            "name": "node",
            "plural": false,
            "selections": [
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "id",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "imageUrl",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "serviceName",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "programName",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "recordingId",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "position",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "broadcastAt",
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
      }
    ],
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": [
      (v0/*:: as any*/),
      (v1/*:: as any*/),
      (v2/*:: as any*/)
    ],
    "kind": "Fragment",
    "metadata": null,
    "name": "capturesListQuery",
    "selections": (v3/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [
      (v1/*:: as any*/),
      (v0/*:: as any*/),
      (v2/*:: as any*/)
    ],
    "kind": "Operation",
    "name": "capturesListQuery",
    "selections": (v3/*:: as any*/)
  },
  "params": {
    "cacheID": "6436c5ddc3d17352b708b44adff4158a",
    "id": null,
    "metadata": {},
    "name": "capturesListQuery",
    "operationKind": "query",
    "text": "query capturesListQuery(\n  $first: Int!\n  $after: String\n  $keyword: String\n) {\n  capturesAvailable\n  captures(first: $first, after: $after, keyword: $keyword) {\n    edges {\n      node {\n        id\n        imageUrl\n        serviceName\n        programName\n        recordingId\n        position\n        broadcastAt\n      }\n    }\n    pageInfo {\n      hasNextPage\n      endCursor\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "1e6c55f75cd65ed7d71e133c3fbcc89b";

export default node;
