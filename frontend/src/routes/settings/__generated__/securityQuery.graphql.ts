/**
 * @generated SignedSource<<3b1ce2adf3d6f8d64098dfec1492c0eb>>
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
  "name": "createdAt",
  "storageKey": null
},
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "lastUsedAt",
  "storageKey": null
},
v2 = [
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
  (v0/*:: as any*/),
  (v1/*:: as any*/)
],
v3 = [
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
    "selections": (v2/*:: as any*/),
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "concreteType": "CalendarFeed",
    "kind": "LinkedField",
    "name": "calendarFeed",
    "plural": false,
    "selections": [
      (v0/*:: as any*/),
      (v1/*:: as any*/)
    ],
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "concreteType": "ApiToken",
    "kind": "LinkedField",
    "name": "apiTokens",
    "plural": true,
    "selections": (v2/*:: as any*/),
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "securityQuery",
    "selections": (v3/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "securityQuery",
    "selections": (v3/*:: as any*/)
  },
  "params": {
    "cacheID": "c64794219f7417d92959d03c2800bbf9",
    "id": null,
    "metadata": {},
    "name": "securityQuery",
    "operationKind": "query",
    "text": "query securityQuery {\n  secondFactors {\n    totp\n    remainingRecoveryCodes\n  }\n  passkeys {\n    id\n    name\n    createdAt\n    lastUsedAt\n  }\n  calendarFeed {\n    createdAt\n    lastUsedAt\n  }\n  apiTokens {\n    id\n    name\n    createdAt\n    lastUsedAt\n  }\n}\n"
  }
};
})();

(node as any).hash = "dae20ed4aa74d163d59d91c0007566f2";

export default node;
