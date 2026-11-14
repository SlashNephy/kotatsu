/**
 * @generated SignedSource<<7a5518517b19b3e727daae8e104a814d>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type Role = "ADMIN" | "MEMBER" | "VIEWER" | "%future added value";
export type LineupBannerViewerQuery$variables = Record<PropertyKey, never>;
export type LineupBannerViewerQuery$data = {
  readonly viewer: {
    readonly role: Role;
  };
};
export type LineupBannerViewerQuery = {
  response: LineupBannerViewerQuery$data;
  variables: LineupBannerViewerQuery$variables;
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
    "name": "LineupBannerViewerQuery",
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
    "name": "LineupBannerViewerQuery",
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
    "cacheID": "3ee2550162b73f8cb578ab6e00eb20b7",
    "id": null,
    "metadata": {},
    "name": "LineupBannerViewerQuery",
    "operationKind": "query",
    "text": "query LineupBannerViewerQuery {\n  viewer {\n    role\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "613c6df7a2cf5d5157fb42da09bb6c1c";

export default node;
