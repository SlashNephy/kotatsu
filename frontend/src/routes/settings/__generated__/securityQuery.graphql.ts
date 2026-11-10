/**
 * @generated SignedSource<<3c5216dbbd1ab5410e425103688a3742>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type securityQuery$variables = Record<PropertyKey, never>;
export type securityQuery$data = {
  readonly apiTokens: ReadonlyArray<{
    readonly createdAt: string;
    readonly id: string;
    readonly lastUsedAt: string | null | undefined;
    readonly name: string;
  }>;
  readonly calendarFeed: {
    readonly createdAt: string;
    readonly lastUsedAt: string | null | undefined;
  } | null | undefined;
  readonly iptvFeed: {
    readonly createdAt: string;
    readonly lastUsedAt: string | null | undefined;
  } | null | undefined;
  readonly passkeys: ReadonlyArray<{
    readonly createdAt: string;
    readonly id: string;
    readonly lastUsedAt: string | null | undefined;
    readonly name: string;
  }>;
  readonly secondFactors: {
    readonly remainingRecoveryCodes: number;
    readonly totp: boolean;
  };
  readonly viewer: {
    readonly username: string;
  };
};
export type securityQuery = {
  response: securityQuery$data;
  variables: securityQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "username",
  "storageKey": null
},
v1 = {
  "alias": null,
  "args": null,
  "concreteType": "SecondFactorStatus",
  "kind": "LinkedField",
  "name": "secondFactors",
  "plural": false,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "totp",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "remainingRecoveryCodes",
      "storageKey": null
    }
  ],
  "storageKey": null
},
v2 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v3 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "createdAt",
  "storageKey": null
},
v4 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "lastUsedAt",
  "storageKey": null
},
v5 = [
  (v2/*:: as any*/),
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "name",
    "storageKey": null
  },
  (v3/*:: as any*/),
  (v4/*:: as any*/)
],
v6 = {
  "alias": null,
  "args": null,
  "concreteType": "Passkey",
  "kind": "LinkedField",
  "name": "passkeys",
  "plural": true,
  "selections": (v5/*:: as any*/),
  "storageKey": null
},
v7 = [
  (v3/*:: as any*/),
  (v4/*:: as any*/)
],
v8 = {
  "alias": null,
  "args": null,
  "concreteType": "CalendarFeed",
  "kind": "LinkedField",
  "name": "calendarFeed",
  "plural": false,
  "selections": (v7/*:: as any*/),
  "storageKey": null
},
v9 = {
  "alias": null,
  "args": null,
  "concreteType": "IptvFeed",
  "kind": "LinkedField",
  "name": "iptvFeed",
  "plural": false,
  "selections": (v7/*:: as any*/),
  "storageKey": null
},
v10 = {
  "alias": null,
  "args": null,
  "concreteType": "ApiToken",
  "kind": "LinkedField",
  "name": "apiTokens",
  "plural": true,
  "selections": (v5/*:: as any*/),
  "storageKey": null
};
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "securityQuery",
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
      (v1/*:: as any*/),
      (v6/*:: as any*/),
      (v8/*:: as any*/),
      (v9/*:: as any*/),
      (v10/*:: as any*/)
    ],
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "securityQuery",
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
          (v2/*:: as any*/)
        ],
        "storageKey": null
      },
      (v1/*:: as any*/),
      (v6/*:: as any*/),
      (v8/*:: as any*/),
      (v9/*:: as any*/),
      (v10/*:: as any*/)
    ]
  },
  "params": {
    "cacheID": "d4440f0c9e9184bac002e54aabd09ef0",
    "id": null,
    "metadata": {},
    "name": "securityQuery",
    "operationKind": "query",
    "text": "query securityQuery {\n  viewer {\n    username\n    id\n  }\n  secondFactors {\n    totp\n    remainingRecoveryCodes\n  }\n  passkeys {\n    id\n    name\n    createdAt\n    lastUsedAt\n  }\n  calendarFeed {\n    createdAt\n    lastUsedAt\n  }\n  iptvFeed {\n    createdAt\n    lastUsedAt\n  }\n  apiTokens {\n    id\n    name\n    createdAt\n    lastUsedAt\n  }\n}\n"
  }
};
})();

(node as any).hash = "f7c836df4cf6bdfa3d0d60d8eaffb595";

export default node;
