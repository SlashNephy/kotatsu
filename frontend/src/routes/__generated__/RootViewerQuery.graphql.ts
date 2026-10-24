/**
 * @generated SignedSource<<6d5ca0a5230c5f36c30113fe920bc876>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type RootViewerQuery$variables = Record<PropertyKey, never>;
export type RootViewerQuery$data = {
  readonly viewer: {
    readonly username: string;
  };
};
export type RootViewerQuery = {
  response: RootViewerQuery$data;
  variables: RootViewerQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "username",
  "storageKey": null
};
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "RootViewerQuery",
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "User",
        "kind": "LinkedField",
        "name": "viewer",
        "plural": false,
        "selections": [
          (v0/*:: as any*/)
        ],
        "storageKey": null
      }
    ],
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "RootViewerQuery",
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "User",
        "kind": "LinkedField",
        "name": "viewer",
        "plural": false,
        "selections": [
          (v0/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "id",
            "storageKey": null
          }
        ],
        "storageKey": null
      }
    ]
  },
  "params": {
    "cacheID": "4cc1d18bf55150a30bd8c98e1435d9d2",
    "id": null,
    "metadata": {},
    "name": "RootViewerQuery",
    "operationKind": "query",
    "text": "query RootViewerQuery {\n  viewer {\n    username\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "c59aba27e4288c7d46a5269ff6a1da67";

export default node;
