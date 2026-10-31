/**
 * @generated SignedSource<<a36bb08b762ce2f6596dfea05faf80f5>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type Role = "ADMIN" | "MEMBER" | "VIEWER" | "%future added value";
export type useDRCSReplacementsQuery$variables = Record<PropertyKey, never>;
export type useDRCSReplacementsQuery$data = {
  readonly drcsReplacements: ReadonlyArray<{
    readonly md5: string;
    readonly text: string;
  }>;
  readonly viewer: {
    readonly role: Role;
  };
};
export type useDRCSReplacementsQuery = {
  response: useDRCSReplacementsQuery$data;
  variables: useDRCSReplacementsQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "alias": null,
  "args": null,
  "concreteType": "DrcsReplacement",
  "kind": "LinkedField",
  "name": "drcsReplacements",
  "plural": true,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "md5",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "text",
      "storageKey": null
    }
  ],
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
    "name": "useDRCSReplacementsQuery",
    "selections": [
      (v0/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "concreteType": "User",
        "kind": "LinkedField",
        "name": "viewer",
        "plural": false,
        "selections": [
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
    "name": "useDRCSReplacementsQuery",
    "selections": [
      (v0/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "concreteType": "User",
        "kind": "LinkedField",
        "name": "viewer",
        "plural": false,
        "selections": [
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
    "cacheID": "148a66bc8d9a8a5a43d3eff4329fcf1f",
    "id": null,
    "metadata": {},
    "name": "useDRCSReplacementsQuery",
    "operationKind": "query",
    "text": "query useDRCSReplacementsQuery {\n  drcsReplacements {\n    md5\n    text\n  }\n  viewer {\n    role\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "15454d1f9811d2c882eb30c68502251c";

export default node;
