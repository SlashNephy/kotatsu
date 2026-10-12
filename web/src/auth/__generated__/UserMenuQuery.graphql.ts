/**
 * @generated SignedSource<<bed0ffa1dc44f4aed06815252fd39d9a>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type UserMenuQuery$variables = Record<PropertyKey, never>;
export type UserMenuQuery$data = {
  readonly viewer: {
    readonly displayName: string;
    readonly username: string;
  };
};
export type UserMenuQuery = {
  response: UserMenuQuery$data;
  variables: UserMenuQuery$variables;
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
  "name": "username",
  "storageKey": null
};
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "UserMenuQuery",
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
    "name": "UserMenuQuery",
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
    "cacheID": "13a02abb99e3dc73246ca57a7621fb16",
    "id": null,
    "metadata": {},
    "name": "UserMenuQuery",
    "operationKind": "query",
    "text": "query UserMenuQuery {\n  viewer {\n    displayName\n    username\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "cc7e8a61120cab9c84a6cb92b219c21b";

export default node;
