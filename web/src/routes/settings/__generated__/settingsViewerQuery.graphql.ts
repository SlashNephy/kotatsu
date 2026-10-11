/**
 * @generated SignedSource<<a7286956d90e70a2f17a2cff93eb7531>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type Role = "ADMIN" | "MEMBER" | "VIEWER" | "%future added value";
export type settingsViewerQuery$variables = Record<PropertyKey, never>;
export type settingsViewerQuery$data = {
  readonly viewer: {
    readonly role: Role;
  };
};
export type settingsViewerQuery = {
  response: settingsViewerQuery$data;
  variables: settingsViewerQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
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
    "name": "settingsViewerQuery",
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
    "name": "settingsViewerQuery",
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
    "cacheID": "9c342c76743f6f2686ba2b5f25527714",
    "id": null,
    "metadata": {},
    "name": "settingsViewerQuery",
    "operationKind": "query",
    "text": "query settingsViewerQuery {\n  viewer {\n    role\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "ebe7edfbbb72622419747495f83af061";

export default node;
