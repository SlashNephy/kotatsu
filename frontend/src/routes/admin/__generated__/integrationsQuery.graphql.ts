/**
 * @generated SignedSource<<2e783d82acb906d7931b5b836d1833c8>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type AnimeImportSource = "ANNICT_WORKS" | "ARM" | "SAYA_DEFINITIONS" | "SYOBOCAL_TITLES" | "%future added value";
export type AnimeRecordDestination = "ANILIST" | "ANNICT" | "%future added value";
export type CommentSource = "NICOLOG" | "NX_JIKKYO" | "%future added value";
export type NotificationKind = "ANIME_RECORDED" | "ANIME_RECORD_FAILED" | "LINEUP_CHANGED" | "RECORDING_COMPLETED" | "RECORDING_FAILED" | "RECORDING_STARTED" | "RESERVATION_CONFLICT" | "SCRAMBLED_RECORDINGS" | "STORAGE_LOW" | "TUNER_SERVER_UNREACHABLE" | "WATCH_REMINDER" | "%future added value";
export type WebhookFormat = "DISCORD" | "JSON" | "SLACK" | "%future added value";
export type integrationsQuery$variables = Record<PropertyKey, never>;
export type integrationsQuery$data = {
  readonly animeImportToken: {
    readonly configured: boolean;
  };
  readonly animeImports: ReadonlyArray<{
    readonly checkedAt: string;
    readonly error: string | null | undefined;
    readonly importedAt: string | null | undefined;
    readonly itemCount: number;
    readonly source: AnimeImportSource;
  }>;
  readonly animeRecordApps: ReadonlyArray<{
    readonly clientId: string | null | undefined;
    readonly configured: boolean;
    readonly destination: AnimeRecordDestination;
    readonly redirectUri: string;
  }>;
  readonly issuedStreamUrls: ReadonlyArray<{
    readonly createdAt: string;
    readonly expiresAt: string;
    readonly id: string;
    readonly lastUsedAt: string | null | undefined;
    readonly live: boolean;
    readonly targetName: string;
    readonly userDisplayName: string;
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
    readonly commentSources: ReadonlyArray<CommentSource>;
    readonly dataBroadcastIpEnabled: boolean;
    readonly epgstationApiEnabled: boolean;
    readonly externalPlayerUrlsEnabled: boolean;
    readonly iptvFeedsEnabled: boolean;
    readonly mcpEnabled: boolean;
    readonly networkActivityEnabled: boolean;
    readonly recordingDownloadsEnabled: boolean;
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
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "configured",
  "storageKey": null
},
v2 = [
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
        "name": "externalPlayerUrlsEnabled",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "iptvFeedsEnabled",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "recordingDownloadsEnabled",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "epgstationApiEnabled",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "networkActivityEnabled",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "animeLinkingEnabled",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "commentSources",
        "storageKey": null
      }
    ],
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "concreteType": "IssuedStreamUrl",
    "kind": "LinkedField",
    "name": "issuedStreamUrls",
    "plural": true,
    "selections": [
      (v0/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "userDisplayName",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "targetName",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "live",
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
        "name": "expiresAt",
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
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "concreteType": "AnimeRecordApp",
    "kind": "LinkedField",
    "name": "animeRecordApps",
    "plural": true,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "destination",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "clientId",
        "storageKey": null
      },
      (v1/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "redirectUri",
        "storageKey": null
      }
    ],
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "concreteType": "AnimeImportToken",
    "kind": "LinkedField",
    "name": "animeImportToken",
    "plural": false,
    "selections": [
      (v1/*:: as any*/)
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
    "selections": (v2/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "integrationsQuery",
    "selections": (v2/*:: as any*/)
  },
  "params": {
    "cacheID": "e0efad40072c896c1b2a7d8b04eccba4",
    "id": null,
    "metadata": {},
    "name": "integrationsQuery",
    "operationKind": "query",
    "text": "query integrationsQuery {\n  systemSettings {\n    mcpEnabled\n    dataBroadcastIpEnabled\n    externalPlayerUrlsEnabled\n    iptvFeedsEnabled\n    recordingDownloadsEnabled\n    epgstationApiEnabled\n    networkActivityEnabled\n    animeLinkingEnabled\n    commentSources\n  }\n  issuedStreamUrls {\n    id\n    userDisplayName\n    targetName\n    live\n    createdAt\n    expiresAt\n    lastUsedAt\n  }\n  animeRecordApps {\n    destination\n    clientId\n    configured\n    redirectUri\n  }\n  animeImportToken {\n    configured\n  }\n  animeImports {\n    source\n    checkedAt\n    importedAt\n    itemCount\n    error\n  }\n  webhooks {\n    id\n    name\n    url\n    format\n    kinds\n    enabled\n  }\n  secondFactors {\n    totp\n    remainingRecoveryCodes\n  }\n  passkeys {\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "47e312694f16915193f9e9cd84fd00cc";

export default node;
