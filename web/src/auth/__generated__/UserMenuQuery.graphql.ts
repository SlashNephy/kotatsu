/**
 * @generated SignedSource<<f1451dd150ae5e54146c95289648c6e5>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type UserMenuQuery$variables = Record<PropertyKey, never>;
export type UserMenuQuery$data = {
  readonly version: {
    readonly modified: boolean;
    readonly revision: string | null | undefined;
    readonly version: string;
  };
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
},
v2 = {
  "alias": null,
  "args": null,
  "concreteType": "AppVersion",
  "kind": "LinkedField",
  "name": "version",
  "plural": false,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "version",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "revision",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "modified",
      "storageKey": null
    }
  ],
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
      },
      (v2/*:: as any*/)
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
      },
      (v2/*:: as any*/)
    ]
  },
  "params": {
    "cacheID": "42643c26da12af7d82ed87937a499216",
    "id": null,
    "metadata": {},
    "name": "UserMenuQuery",
    "operationKind": "query",
    "text": "query UserMenuQuery {\n  viewer {\n    displayName\n    username\n    id\n  }\n  version {\n    version\n    revision\n    modified\n  }\n}\n"
  }
};
})();

(node as any).hash = "74e37d7c5f1f419e74faa04efa00db8c";

export default node;
