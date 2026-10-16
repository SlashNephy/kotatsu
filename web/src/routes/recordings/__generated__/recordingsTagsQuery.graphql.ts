/**
 * @generated SignedSource<<99a10f3ee84ad4255c1a22750a648667>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
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
  "name": "id",
  "storageKey": null
},
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v2 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "Tag",
    "kind": "LinkedField",
    "name": "tags",
    "plural": true,
    "selections": [
      (v0/*:: as any*/),
      (v1/*:: as any*/),
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
  {
    "alias": null,
    "args": null,
    "concreteType": "Service",
    "kind": "LinkedField",
    "name": "services",
    "plural": true,
    "selections": [
      (v0/*:: as any*/),
      (v1/*:: as any*/)
    ],
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "recordingsTagsQuery",
    "selections": (v2/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "recordingsTagsQuery",
    "selections": (v2/*:: as any*/)
  },
  "params": {
    "cacheID": "bfe5a16aac30023060289c5530e11a6b",
    "id": null,
    "metadata": {},
    "name": "recordingsTagsQuery",
    "operationKind": "query",
    "text": "query recordingsTagsQuery {\n  tags {\n    id\n    name\n    recordings\n  }\n  services {\n    id\n    name\n  }\n}\n"
  }
};
})();

(node as any).hash = "67fc8857ea541fc60e00b29469244de6";

export default node;
