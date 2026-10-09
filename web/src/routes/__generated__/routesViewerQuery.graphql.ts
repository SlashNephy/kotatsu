/**
 * @generated SignedSource<<c3c10974a228338cd4c435cfbe87b8fc>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type Role = "ADMIN" | "MEMBER" | "VIEWER" | "%future added value";
export type routesViewerQuery$variables = Record<PropertyKey, never>;
export type routesViewerQuery$data = {
  readonly viewer: {
    readonly displayName: string;
    readonly role: Role;
  };
};
export type routesViewerQuery = {
  response: routesViewerQuery$data;
  variables: routesViewerQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "displayName",
  "storageKey": null
},
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "role",
  "storageKey": null
};
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "routesViewerQuery",
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
          (v1/*:: as any*/)
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
    "name": "routesViewerQuery",
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
          (v1/*:: as any*/),
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
    "cacheID": "d7fcda6d3be52fc01230636bdc637572",
    "id": null,
    "metadata": {},
    "name": "routesViewerQuery",
    "operationKind": "query",
    "text": "query routesViewerQuery {\n  viewer {\n    displayName\n    role\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "53e183f8d38a6ac40add6ffc46fc2ab2";

export default node;
