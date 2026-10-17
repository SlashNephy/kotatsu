/**
 * @generated SignedSource<<590c1504fb164a0f78916140179eca87>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type Role = "ADMIN" | "MEMBER" | "VIEWER" | "%future added value";
export type recordingsTagsQuery$variables = Record<PropertyKey, never>;
export type recordingsTagsQuery$data = {
  readonly services: ReadonlyArray<{
    readonly id: string;
    readonly name: string;
  }>;
  readonly tags: ReadonlyArray<{
    readonly id: string;
    readonly name: string;
    readonly recordings: number;
  }>;
  readonly viewer: {
    readonly role: Role;
  };
};
export type recordingsTagsQuery = {
  response: recordingsTagsQuery$data;
  variables: recordingsTagsQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "role",
  "storageKey": null
},
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v2 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v3 = {
  "alias": null,
  "args": null,
  "concreteType": "Tag",
  "kind": "LinkedField",
  "name": "tags",
  "plural": true,
  "selections": [
    (v1/*:: as any*/),
    (v2/*:: as any*/),
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "recordings",
      "storageKey": null
    }
  ],
  "storageKey": null
},
v4 = {
  "alias": null,
  "args": null,
  "concreteType": "Service",
  "kind": "LinkedField",
  "name": "services",
  "plural": true,
  "selections": [
    (v1/*:: as any*/),
    (v2/*:: as any*/)
  ],
  "storageKey": null
};
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "recordingsTagsQuery",
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
      },
      (v3/*:: as any*/),
      (v4/*:: as any*/)
    ],
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "recordingsTagsQuery",
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
      (v3/*:: as any*/),
      (v4/*:: as any*/)
    ]
  },
  "params": {
    "cacheID": "410875bbbd0967f7abd6da3eca75fbb6",
    "id": null,
    "metadata": {},
    "name": "recordingsTagsQuery",
    "operationKind": "query",
    "text": "query recordingsTagsQuery {\n  viewer {\n    role\n    id\n  }\n  tags {\n    id\n    name\n    recordings\n  }\n  services {\n    id\n    name\n  }\n}\n"
  }
};
})();

(node as any).hash = "0c92c5e9b45e5746ec4cbc9c02578402";

export default node;
