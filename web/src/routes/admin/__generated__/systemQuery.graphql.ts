/**
 * @generated SignedSource<<f46f3b124041d5c2540aef5895b52d07>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type systemQuery$variables = Record<PropertyKey, never>;
export type systemQuery$data = {
  readonly channelTypes: ReadonlyArray<{
    readonly displayName: string;
    readonly sortOrder: number;
    readonly type: string;
  }>;
  readonly passkeys: ReadonlyArray<{
    readonly id: string;
  }>;
  readonly secondFactors: {
    readonly remainingRecoveryCodes: number;
    readonly totp: boolean;
  };
  readonly services: ReadonlyArray<{
    readonly channelType: {
      readonly displayName: string;
    };
    readonly hidden: boolean;
    readonly id: string;
    readonly name: string;
    readonly sortOrder: number | null | undefined;
  }>;
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
    }>;
    readonly tunerServer: {
      readonly checkedAt: string;
      readonly clockSkewMs: number | null | undefined;
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
  "name": "name",
  "storageKey": null
},
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v2 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "displayName",
  "storageKey": null
},
v3 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "sortOrder",
  "storageKey": null
},
v4 = [
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
          (v0/*:: as any*/),
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
          (v0/*:: as any*/),
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
          (v1/*:: as any*/),
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
    "concreteType": "ChannelType",
    "kind": "LinkedField",
    "name": "channelTypes",
    "plural": true,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "type",
        "storageKey": null
      },
      (v2/*:: as any*/),
      (v3/*:: as any*/)
    ],
    "storageKey": null
  },
  {
    "alias": null,
    "args": [
      {
        "kind": "Literal",
        "name": "includeHidden",
        "value": true
      }
    ],
    "concreteType": "Service",
    "kind": "LinkedField",
    "name": "services",
    "plural": true,
    "selections": [
      (v1/*:: as any*/),
      (v0/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "hidden",
        "storageKey": null
      },
      (v3/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "concreteType": "ChannelType",
        "kind": "LinkedField",
        "name": "channelType",
        "plural": false,
        "selections": [
          (v2/*:: as any*/)
        ],
        "storageKey": null
      }
    ],
    "storageKey": "services(includeHidden:true)"
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
      (v1/*:: as any*/)
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
    "selections": (v4/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "systemQuery",
    "selections": (v4/*:: as any*/)
  },
  "params": {
    "cacheID": "1b7e259eecf97c33b009a46002e797a5",
    "id": null,
    "metadata": {},
    "name": "systemQuery",
    "operationKind": "query",
    "text": "query systemQuery {\n  systemStatus {\n    instances {\n      instanceId\n      role\n      address\n      startedAt\n      heartbeatAt\n      alive\n    }\n    leases {\n      name\n      holderInstanceId\n      generation\n      expiresAt\n      valid\n    }\n    tunerServer {\n      degraded\n      degradedSince\n      lastError\n      clockSkewMs\n      checkedAt\n    }\n    tuners {\n      index\n      name\n      types\n      available\n      remote\n      fault\n    }\n    pools {\n      id\n      path\n      priority\n      minFreeBytes\n      freeBytes\n    }\n  }\n  channelTypes {\n    type\n    displayName\n    sortOrder\n  }\n  services(includeHidden: true) {\n    id\n    name\n    hidden\n    sortOrder\n    channelType {\n      displayName\n    }\n  }\n  secondFactors {\n    totp\n    remainingRecoveryCodes\n  }\n  passkeys {\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "a4b9f55099e8573ece0a228b91f2a611";

export default node;
