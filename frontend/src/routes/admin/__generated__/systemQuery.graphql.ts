/**
 * @generated SignedSource<<8f3787bbd958f6a454c75803de89547b>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type systemQuery$variables = Record<PropertyKey, never>;
export type systemQuery$data = {
  readonly passkeys: ReadonlyArray<{
    readonly id: string;
  }>;
  readonly secondFactors: {
    readonly remainingRecoveryCodes: number;
    readonly totp: boolean;
  };
  readonly systemStatus: {
    readonly instances: ReadonlyArray<{
      readonly address: string;
      readonly alive: boolean;
      readonly heartbeatAt: string;
      readonly instanceId: string;
      readonly role: string;
      readonly startedAt: string;
    }>;
    readonly leases: ReadonlyArray<{
      readonly expiresAt: string;
      readonly generation: number;
      readonly holderInstanceId: string;
      readonly name: string;
      readonly valid: boolean;
    }>;
    readonly pools: ReadonlyArray<{
      readonly freeBytes: number | null | undefined;
      readonly id: string;
      readonly minFreeBytes: number;
      readonly path: string;
      readonly priority: number;
      readonly totalBytes: number | null | undefined;
    }>;
    readonly tunerServer: {
      readonly checkedAt: string;
      readonly clockSkewMs: number | null | undefined;
      readonly clockSkewThresholdMs: number;
      readonly clockSkewed: boolean;
      readonly degraded: boolean;
      readonly degradedSince: string | null | undefined;
      readonly lastError: string;
    } | null | undefined;
    readonly tuners: ReadonlyArray<{
      readonly available: boolean;
      readonly fault: boolean;
      readonly index: number;
      readonly name: string;
      readonly remote: boolean;
      readonly types: ReadonlyArray<string>;
    }>;
  };
  readonly version: {
    readonly latestRelease: {
      readonly publishedAt: string;
      readonly url: string;
      readonly version: string;
    } | null | undefined;
    readonly version: string;
  };
};
export type systemQuery = {
  response: systemQuery$data;
  variables: systemQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "version",
  "storageKey": null
},
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v2 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v3 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "AppVersion",
    "kind": "LinkedField",
    "name": "version",
    "plural": false,
    "selections": [
      (v0/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "concreteType": "Release",
        "kind": "LinkedField",
        "name": "latestRelease",
        "plural": false,
        "selections": [
          (v0/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "url",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "publishedAt",
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
    "concreteType": "SystemStatus",
    "kind": "LinkedField",
    "name": "systemStatus",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "Instance",
        "kind": "LinkedField",
        "name": "instances",
        "plural": true,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "instanceId",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "role",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "address",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "startedAt",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "heartbeatAt",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "alive",
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "Lease",
        "kind": "LinkedField",
        "name": "leases",
        "plural": true,
        "selections": [
          (v1/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "holderInstanceId",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "generation",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "expiresAt",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "valid",
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "TunerServerStatus",
        "kind": "LinkedField",
        "name": "tunerServer",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "degraded",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "degradedSince",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "lastError",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "clockSkewMs",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "clockSkewed",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "clockSkewThresholdMs",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "checkedAt",
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "Tuner",
        "kind": "LinkedField",
        "name": "tuners",
        "plural": true,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "index",
            "storageKey": null
          },
          (v1/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "types",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "available",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "remote",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "fault",
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "StoragePool",
        "kind": "LinkedField",
        "name": "pools",
        "plural": true,
        "selections": [
          (v2/*:: as any*/),
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
      (v2/*:: as any*/)
    ],
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "systemQuery",
    "selections": (v3/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "systemQuery",
    "selections": (v3/*:: as any*/)
  },
  "params": {
    "cacheID": "adc855c3c52e4dc0db52dfda21b61cf0",
    "id": null,
    "metadata": {},
    "name": "systemQuery",
    "operationKind": "query",
    "text": "query systemQuery {\n  version {\n    version\n    latestRelease {\n      version\n      url\n      publishedAt\n    }\n  }\n  systemStatus {\n    instances {\n      instanceId\n      role\n      address\n      startedAt\n      heartbeatAt\n      alive\n    }\n    leases {\n      name\n      holderInstanceId\n      generation\n      expiresAt\n      valid\n    }\n    tunerServer {\n      degraded\n      degradedSince\n      lastError\n      clockSkewMs\n      clockSkewed\n      clockSkewThresholdMs\n      checkedAt\n    }\n    tuners {\n      index\n      name\n      types\n      available\n      remote\n      fault\n    }\n    pools {\n      id\n      path\n      priority\n      minFreeBytes\n      freeBytes\n      totalBytes\n    }\n  }\n  secondFactors {\n    totp\n    remainingRecoveryCodes\n  }\n  passkeys {\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "a8ff6e8d7d10c95dbec31b2a438f48b7";

export default node;
