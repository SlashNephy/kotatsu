/**
 * @generated SignedSource<<aba2f394a60c6e026e5214cf640a4ea8>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type Role = "ADMIN" | "MEMBER" | "VIEWER" | "%future added value";
export type RootViewerQuery$variables = Record<PropertyKey, never>;
export type RootViewerQuery$data = {
  readonly viewer: {
    readonly role: Role;
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
    "cacheID": "c049c5c10195dbba94e281c6edd12d64",
    "id": null,
    "metadata": {},
    "name": "RootViewerQuery",
    "operationKind": "query",
    "text": "query RootViewerQuery {\n  viewer {\n    username\n    role\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "e11f1e95ab4cef4a7d45a5acb0350aec";

export default node;
