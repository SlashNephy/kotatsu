/**
 * @generated SignedSource<<4752b7e76fd736112e6e4d133221c9e7>>
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
var v0 = [
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
    "name": "createdAt",
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "lastUsedAt",
    "storageKey": null
  }
],
v1 = [
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
    "selections": (v0/*:: as any*/),
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "concreteType": "ApiToken",
    "kind": "LinkedField",
    "name": "apiTokens",
    "plural": true,
    "selections": (v0/*:: as any*/),
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "securityQuery",
    "selections": (v1/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "securityQuery",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "071e558b88ecd896cd11333171d2da15",
    "id": null,
    "metadata": {},
    "name": "securityQuery",
    "operationKind": "query",
    "text": "query securityQuery {\n  secondFactors {\n    totp\n    remainingRecoveryCodes\n  }\n  passkeys {\n    id\n    name\n    createdAt\n    lastUsedAt\n  }\n  apiTokens {\n    id\n    name\n    createdAt\n    lastUsedAt\n  }\n}\n"
  }
};
})();

(node as any).hash = "3cd273a6903fb8203c154d7457b7d96e";

export default node;
