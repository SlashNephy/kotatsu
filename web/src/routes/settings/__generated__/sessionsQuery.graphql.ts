/**
 * @generated SignedSource<<a3eaf817ff9d7cdfbe3c8425f0cf9e3c>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type sessionsQuery$variables = Record<PropertyKey, never>;
export type sessionsQuery$data = {
  readonly sessions: ReadonlyArray<{
    readonly createdAt: string;
    readonly current: boolean;
    readonly id: string;
    readonly lastSeenAt: string;
    readonly sourceIp: string | null | undefined;
    readonly userAgent: string;
  }>;
};
export type sessionsQuery = {
  response: sessionsQuery$data;
  variables: sessionsQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "Session",
    "kind": "LinkedField",
    "name": "sessions",
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
        "name": "current",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "createdAt",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "lastSeenAt",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "userAgent",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "sourceIp",
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
    "name": "sessionsQuery",
    "selections": (v0/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "sessionsQuery",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "818c484c9c7a81caaa3f3be2f5101200",
    "id": null,
    "metadata": {},
    "name": "sessionsQuery",
    "operationKind": "query",
    "text": "query sessionsQuery {\n  sessions {\n    id\n    current\n    createdAt\n    lastSeenAt\n    userAgent\n    sourceIp\n  }\n}\n"
  }
};
})();

(node as any).hash = "c9024fb206eeaffacb1a8dd79e2a739e";

export default node;
