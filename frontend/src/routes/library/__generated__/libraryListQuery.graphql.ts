/**
 * @generated SignedSource<<4f9011596f139b91f9a5b6ffa014fa71>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type LibraryKind = "FOLDER" | "POOL" | "%future added value";
export type libraryListQuery$variables = Record<PropertyKey, never>;
export type libraryListQuery$data = {
  readonly libraries: ReadonlyArray<{
    readonly kind: LibraryKind;
    readonly name: string;
  }>;
};
export type libraryListQuery = {
  response: libraryListQuery$data;
  variables: libraryListQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "Library",
    "kind": "LinkedField",
    "name": "libraries",
    "plural": true,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "name",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "kind",
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
    "name": "libraryListQuery",
    "selections": (v0/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "libraryListQuery",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "696b20bdac19ce42c11860552b85fbfd",
    "id": null,
    "metadata": {},
    "name": "libraryListQuery",
    "operationKind": "query",
    "text": "query libraryListQuery {\n  libraries {\n    name\n    kind\n  }\n}\n"
  }
};
})();

(node as any).hash = "f3ab4a2f1cd4e8f8f89ba1f2c508f16f";

export default node;
