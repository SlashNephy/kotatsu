/**
 * @generated SignedSource<<f9b2bac56b21e4dcb9f0eee7bb423611>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type storageQuery$variables = Record<PropertyKey, never>;
export type storageQuery$data = {
  readonly systemStatus: {
    readonly pools: ReadonlyArray<{
      readonly freeBytes: number | null | undefined;
      readonly id: string;
      readonly minFreeBytes: number;
      readonly path: string;
      readonly priority: number;
      readonly totalBytes: number | null | undefined;
    }>;
  };
};
export type storageQuery = {
  response: storageQuery$data;
  variables: storageQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "SystemStatus",
    "kind": "LinkedField",
    "name": "systemStatus",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "StoragePool",
        "kind": "LinkedField",
        "name": "pools",
        "plural": true,
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
            "name": "path",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "priority",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "minFreeBytes",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "freeBytes",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "totalBytes",
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
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "storageQuery",
    "selections": (v0/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "storageQuery",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "7527bf343eb2aef5c8561cd4bf120459",
    "id": null,
    "metadata": {},
    "name": "storageQuery",
    "operationKind": "query",
    "text": "query storageQuery {\n  systemStatus {\n    pools {\n      id\n      path\n      priority\n      minFreeBytes\n      freeBytes\n      totalBytes\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "1583f67f3ee944ee22b77bf522d75265";

export default node;
