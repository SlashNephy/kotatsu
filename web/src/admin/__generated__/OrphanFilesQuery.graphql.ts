/**
 * @generated SignedSource<<0e4a1a7a14a4aae99a5cc4ddd756273c>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type OrphanFilesQuery$variables = Record<PropertyKey, never>;
export type OrphanFilesQuery$data = {
  readonly orphanFiles: ReadonlyArray<{
    readonly modifiedAt: string;
    readonly path: string;
    readonly poolId: string;
    readonly sidecar: boolean;
    readonly size: number;
  }>;
};
export type OrphanFilesQuery = {
  response: OrphanFilesQuery$data;
  variables: OrphanFilesQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "OrphanFile",
    "kind": "LinkedField",
    "name": "orphanFiles",
    "plural": true,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "poolId",
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
        "name": "sidecar",
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
        "name": "modifiedAt",
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
    "name": "OrphanFilesQuery",
    "selections": (v0/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "OrphanFilesQuery",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "7b09863da2eac74eb0df05b7961596ac",
    "id": null,
    "metadata": {},
    "name": "OrphanFilesQuery",
    "operationKind": "query",
    "text": "query OrphanFilesQuery {\n  orphanFiles {\n    poolId\n    path\n    sidecar\n    size\n    modifiedAt\n  }\n}\n"
  }
};
})();

(node as any).hash = "87bb7d9747dc9ebfb893442e81ba6e32";

export default node;
