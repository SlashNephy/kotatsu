/**
 * @generated SignedSource<<40f587368f204748e7751f758fba1002>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type sessionsQuery$variables = Record<PropertyKey, never>;
export type sessionsQuery$data = {
  readonly oauthGrants: ReadonlyArray<{
    readonly clientId: string;
    readonly clientName: string;
    readonly createdAt: string;
    readonly id: string;
    readonly lastUsedAt: string | null | undefined;
    readonly scopes: ReadonlyArray<string>;
  }>;
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
  "name": "createdAt",
  "storageKey": null
},
v2 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "Session",
    "kind": "LinkedField",
    "name": "sessions",
    "plural": true,
    "selections": [
      (v0/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "current",
        "storageKey": null
      },
      (v1/*:: as any*/),
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
  },
  {
    "alias": null,
    "args": null,
    "concreteType": "OAuthGrant",
    "kind": "LinkedField",
    "name": "oauthGrants",
    "plural": true,
    "selections": [
      (v0/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "clientId",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "clientName",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "scopes",
        "storageKey": null
      },
      (v1/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "lastUsedAt",
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
    "selections": (v2/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "sessionsQuery",
    "selections": (v2/*:: as any*/)
  },
  "params": {
    "cacheID": "3604e3f660e7a79b8b7daff6a53b52ea",
    "id": null,
    "metadata": {},
    "name": "sessionsQuery",
    "operationKind": "query",
    "text": "query sessionsQuery {\n  sessions {\n    id\n    current\n    createdAt\n    lastSeenAt\n    userAgent\n    sourceIp\n  }\n  oauthGrants {\n    id\n    clientId\n    clientName\n    scopes\n    createdAt\n    lastUsedAt\n  }\n}\n"
  }
};
})();

(node as any).hash = "ed4c0ed2a23649bdf4bba2cabf887fab";

export default node;
