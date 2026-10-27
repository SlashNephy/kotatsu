/**
 * @generated SignedSource<<5c3f0326a28fc6ed25ac3257b402a0c3>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type retentionQuery$variables = Record<PropertyKey, never>;
export type retentionQuery$data = {
  readonly passkeys: ReadonlyArray<{
    readonly id: string;
  }>;
  readonly secondFactors: {
    readonly remainingRecoveryCodes: number;
    readonly totp: boolean;
  };
  readonly systemSettings: {
    readonly archiveRetentionDays: number;
    readonly notificationRetentionDays: number;
    readonly updatedAt: string | null | undefined;
  };
};
export type retentionQuery = {
  response: retentionQuery$data;
  variables: retentionQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "SystemSettings",
    "kind": "LinkedField",
    "name": "systemSettings",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "archiveRetentionDays",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "notificationRetentionDays",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "updatedAt",
        "storageKey": null
      }
    ],
    "storageKey": null
  },
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
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "id",
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
    "name": "retentionQuery",
    "selections": (v0/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "retentionQuery",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "cdee110efa49127a40b67b37139d5313",
    "id": null,
    "metadata": {},
    "name": "retentionQuery",
    "operationKind": "query",
    "text": "query retentionQuery {\n  systemSettings {\n    archiveRetentionDays\n    notificationRetentionDays\n    updatedAt\n  }\n  secondFactors {\n    totp\n    remainingRecoveryCodes\n  }\n  passkeys {\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "19b0f78b10c66b43871a1b3c57b96dc4";

export default node;
