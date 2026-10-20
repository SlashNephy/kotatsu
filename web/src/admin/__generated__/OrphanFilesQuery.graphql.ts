/**
 * @generated SignedSource<<6e889d2ae76a4ceaea5290238a2c76da>>
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
    readonly poolId: string | null | undefined;
    readonly segmentDir: boolean;
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
        "name": "segmentDir",
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
    "cacheID": "3daaf20f4b0be859e00711a112e714e8",
    "id": null,
    "metadata": {},
    "name": "OrphanFilesQuery",
    "operationKind": "query",
    "text": "query OrphanFilesQuery {\n  orphanFiles {\n    poolId\n    path\n    segmentDir\n    size\n    modifiedAt\n  }\n}\n"
  }
};
})();

(node as any).hash = "1b3cfb4a05ec46e23d61d8d2b7374911";

export default node;
