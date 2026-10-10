/**
 * @generated SignedSource<<73657291fe4026e78fa55a8f844f06bb>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type recordingsTagsQuery$variables = Record<PropertyKey, never>;
export type recordingsTagsQuery$data = {
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
var v0 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "Tag",
    "kind": "LinkedField",
    "name": "tags",
    "plural": true,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "id",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "name",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "recordings",
        "storageKey": null
      }
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
    "selections": (v0/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "recordingsTagsQuery",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "a0da77e1043475760b7d5ec58b426fa9",
    "id": null,
    "metadata": {},
    "name": "recordingsTagsQuery",
    "operationKind": "query",
    "text": "query recordingsTagsQuery {\n  tags {\n    id\n    name\n    recordings\n  }\n}\n"
  }
};
})();

(node as any).hash = "90df20aaa3f0f4b86bf6a4dc13aac4e1";

export default node;
