/**
 * @generated SignedSource<<c2a953a8eb540ee658d6729ff353fb1d>>
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
  (v0/*:: as any*/),
  (v1/*:: as any*/)
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
    "selections": (v3/*:: as any*/),
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "concreteType": "IptvFeed",
    "kind": "LinkedField",
    "name": "iptvFeed",
    "plural": false,
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
    "selections": (v4/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "securityQuery",
    "selections": (v4/*:: as any*/)
  },
  "params": {
    "cacheID": "1c6358ef4e938040e6773e27c9ea2fa1",
    "id": null,
    "metadata": {},
    "name": "securityQuery",
    "operationKind": "query",
    "text": "query securityQuery {\n  secondFactors {\n    totp\n    remainingRecoveryCodes\n  }\n  passkeys {\n    id\n    name\n    createdAt\n    lastUsedAt\n  }\n  calendarFeed {\n    createdAt\n    lastUsedAt\n  }\n  iptvFeed {\n    createdAt\n    lastUsedAt\n  }\n  apiTokens {\n    id\n    name\n    createdAt\n    lastUsedAt\n  }\n}\n"
  }
};
})();

(node as any).hash = "0af71b31a641e895bff8f671bf10ae00";

export default node;
