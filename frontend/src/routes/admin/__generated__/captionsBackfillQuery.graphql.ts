/**
 * @generated SignedSource<<878e1ade73344e7ca78d4e4b37898009>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type captionsBackfillQuery$variables = Record<PropertyKey, never>;
export type captionsBackfillQuery$data = {
  readonly aribCaptionBackfill: {
    readonly pending: number;
    readonly queued: number;
  };
};
export type captionsBackfillQuery = {
  response: captionsBackfillQuery$data;
  variables: captionsBackfillQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "ARIBCaptionBackfill",
    "kind": "LinkedField",
    "name": "aribCaptionBackfill",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "pending",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "queued",
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
    "name": "captionsBackfillQuery",
    "selections": (v0/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "captionsBackfillQuery",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "bfcd5cc21d17abc8ff2b65b0032b1a37",
    "id": null,
    "metadata": {},
    "name": "captionsBackfillQuery",
    "operationKind": "query",
    "text": "query captionsBackfillQuery {\n  aribCaptionBackfill {\n    pending\n    queued\n  }\n}\n"
  }
};
})();

(node as any).hash = "e7903f388bddf9d9aac3eb1f0034126e";

export default node;
