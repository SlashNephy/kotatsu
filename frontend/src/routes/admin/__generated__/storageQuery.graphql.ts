/**
 * @generated SignedSource<<a8b9e926df8d8715efbbc19e83cb09ae>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type storageQuery$variables = Record<PropertyKey, never>;
export type storageQuery$data = {
  readonly passkeys: ReadonlyArray<{
    readonly id: string;
  }>;
  readonly secondFactors: {
    readonly remainingRecoveryCodes: number;
    readonly totp: boolean;
  };
  readonly systemStatus: {
    readonly pools: ReadonlyArray<{
      readonly freeBytes: number | null | undefined;
      readonly id: string;
      readonly minFreeBytes: number;
      readonly path: string;
      readonly priority: number;
      readonly totalBytes: number | null | undefined;
    }>;
  };
};
export type storageQuery = {
  response: storageQuery$data;
  variables: storageQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v1 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "SystemStatus",
    "kind": "LinkedField",
    "name": "systemStatus",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "StoragePool",
        "kind": "LinkedField",
        "name": "pools",
        "plural": true,
        "selections": [
          (v0/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "path",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "priority",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "minFreeBytes",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "freeBytes",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "totalBytes",
            "storageKey": null
          }
        ],
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
      (v0/*:: as any*/)
    ],
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "storageQuery",
    "selections": (v1/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "storageQuery",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "460fd739e9774f0844c787ec253b2e46",
    "id": null,
    "metadata": {},
    "name": "storageQuery",
    "operationKind": "query",
    "text": "query storageQuery {\n  systemStatus {\n    pools {\n      id\n      path\n      priority\n      minFreeBytes\n      freeBytes\n      totalBytes\n    }\n  }\n  secondFactors {\n    totp\n    remainingRecoveryCodes\n  }\n  passkeys {\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "eaaa4bfd56a47a42c3d6b06af2338085";

export default node;
