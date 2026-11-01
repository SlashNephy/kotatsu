/**
 * @generated SignedSource<<d3928de84b777a21a13245059b1aaf4f>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type AnimeImportSource = "ARM" | "ARM_SUPPLEMENTARY" | "SAYA_DEFINITIONS" | "SYOBOCAL_TITLES" | "%future added value";
export type NotificationKind = "RECORDING_COMPLETED" | "RECORDING_FAILED" | "RECORDING_STARTED" | "RESERVATION_CONFLICT" | "STORAGE_LOW" | "TUNER_SERVER_UNREACHABLE" | "WATCH_REMINDER" | "%future added value";
export type WebhookFormat = "DISCORD" | "JSON" | "SLACK" | "%future added value";
export type integrationsQuery$variables = Record<PropertyKey, never>;
export type integrationsQuery$data = {
  readonly animeImports: ReadonlyArray<{
    readonly checkedAt: string;
    readonly error: string | null | undefined;
    readonly importedAt: string | null | undefined;
    readonly itemCount: number;
    readonly source: AnimeImportSource;
  }>;
  readonly passkeys: ReadonlyArray<{
    readonly id: string;
  }>;
  readonly secondFactors: {
    readonly remainingRecoveryCodes: number;
    readonly totp: boolean;
  };
  readonly systemSettings: {
    readonly animeLinkingEnabled: boolean;
    readonly dataBroadcastIpEnabled: boolean;
    readonly mcpEnabled: boolean;
  };
  readonly webhooks: ReadonlyArray<{
    readonly enabled: boolean;
    readonly format: WebhookFormat;
    readonly id: string;
    readonly kinds: ReadonlyArray<NotificationKind>;
    readonly name: string;
    readonly url: string;
  }>;
};
export type integrationsQuery = {
  response: integrationsQuery$data;
  variables: integrationsQuery$variables;
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
    "concreteType": "SystemSettings",
    "kind": "LinkedField",
    "name": "systemSettings",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "mcpEnabled",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "dataBroadcastIpEnabled",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "animeLinkingEnabled",
        "storageKey": null
      }
    ],
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "concreteType": "AnimeImport",
    "kind": "LinkedField",
    "name": "animeImports",
    "plural": true,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "source",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "checkedAt",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "importedAt",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "itemCount",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "error",
        "storageKey": null
      }
    ],
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "concreteType": "Webhook",
    "kind": "LinkedField",
    "name": "webhooks",
    "plural": true,
    "selections": [
      (v0/*:: as any*/),
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
        "name": "url",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "format",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "kinds",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "enabled",
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
    "name": "integrationsQuery",
    "selections": (v1/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "integrationsQuery",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "25a73c6ed9ee8c1549908bb473aa18dc",
    "id": null,
    "metadata": {},
    "name": "integrationsQuery",
    "operationKind": "query",
    "text": "query integrationsQuery {\n  systemSettings {\n    mcpEnabled\n    dataBroadcastIpEnabled\n    animeLinkingEnabled\n  }\n  animeImports {\n    source\n    checkedAt\n    importedAt\n    itemCount\n    error\n  }\n  webhooks {\n    id\n    name\n    url\n    format\n    kinds\n    enabled\n  }\n  secondFactors {\n    totp\n    remainingRecoveryCodes\n  }\n  passkeys {\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "175be0dee2fe7ab849c45271f42f2eae";

export default node;
