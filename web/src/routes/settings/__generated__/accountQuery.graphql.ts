/**
 * @generated SignedSource<<feedb826eec9ae06f896efff7b081428>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type accountQuery$variables = Record<PropertyKey, never>;
export type accountQuery$data = {
  readonly apiTokens: ReadonlyArray<{
    readonly createdAt: string;
    readonly id: string;
    readonly lastUsedAt: string | null | undefined;
    readonly name: string;
  }>;
  readonly oauthGrants: ReadonlyArray<{
    readonly clientId: string;
    readonly clientName: string;
    readonly createdAt: string;
    readonly id: string;
    readonly lastUsedAt: string | null | undefined;
    readonly scopes: ReadonlyArray<string>;
  }>;
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
  readonly sessions: ReadonlyArray<{
    readonly createdAt: string;
    readonly current: boolean;
    readonly id: string;
    readonly lastSeenAt: string;
    readonly sourceIp: string | null | undefined;
    readonly userAgent: string;
  }>;
};
export type accountQuery = {
  response: accountQuery$data;
  variables: accountQuery$variables;
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
v2 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "lastUsedAt",
  "storageKey": null
},
v3 = [
  (v0/*:: as any*/),
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "name",
    "storageKey": null
  },
  (v1/*:: as any*/),
  (v2/*:: as any*/)
],
v4 = [
  {
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
  {
    "alias": null,
    "args": null,
    "concreteType": "Passkey",
    "kind": "LinkedField",
    "name": "passkeys",
    "plural": true,
    "selections": (v3/*:: as any*/),
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "concreteType": "ApiToken",
    "kind": "LinkedField",
    "name": "apiTokens",
    "plural": true,
    "selections": (v3/*:: as any*/),
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
      (v2/*:: as any*/)
    ],
    "storageKey": null
  },
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
  }
];
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "accountQuery",
    "selections": (v4/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "accountQuery",
    "selections": (v4/*:: as any*/)
  },
  "params": {
    "cacheID": "fac9c0a54a2b2d85288b2b228a92c4b5",
    "id": null,
    "metadata": {},
    "name": "accountQuery",
    "operationKind": "query",
    "text": "query accountQuery {\n  secondFactors {\n    totp\n    remainingRecoveryCodes\n  }\n  passkeys {\n    id\n    name\n    createdAt\n    lastUsedAt\n  }\n  apiTokens {\n    id\n    name\n    createdAt\n    lastUsedAt\n  }\n  oauthGrants {\n    id\n    clientId\n    clientName\n    scopes\n    createdAt\n    lastUsedAt\n  }\n  sessions {\n    id\n    current\n    createdAt\n    lastSeenAt\n    userAgent\n    sourceIp\n  }\n}\n"
  }
};
})();

(node as any).hash = "9ff5456fc9ef208ec51f9f94e226372a";

export default node;
