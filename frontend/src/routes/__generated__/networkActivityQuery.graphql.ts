/**
 * @generated SignedSource<<548c896dd559789d90a5f276426c0f60>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type NetworkActivityInitiator = "SCHEDULED" | "USER" | "%future added value";
export type NetworkActivityPurpose = "ANIME_ARTWORKS" | "ANIME_IMAGE" | "ANIME_LINK" | "ANIME_MAPPINGS" | "ANIME_PROGRAM_SYNC" | "ANIME_RECORD" | "ANIME_WORK_IDS" | "DATA_BROADCAST" | "HIBP" | "JIKKYO_CHANNELS" | "JIKKYO_COMMENTS" | "JIKKYO_KAKOLOG" | "MCP_CLIENT_METADATA" | "NICOLOG_COMMENTS" | "OIDC" | "TELEMETRY_METRICS" | "TELEMETRY_TRACES" | "UPDATE_CHECK" | "USER_AVATAR" | "WEBHOOK" | "WEB_PUSH" | "%future added value";
export type Role = "ADMIN" | "MEMBER" | "VIEWER" | "%future added value";
export type networkActivityQuery$variables = Record<PropertyKey, never>;
export type networkActivityQuery$data = {
  readonly networkActivity: {
    readonly enabled: boolean;
    readonly enabledAt: string | null | undefined;
    readonly entries: ReadonlyArray<{
      readonly count: number;
      readonly host: string | null | undefined;
      readonly hour: string;
      readonly initiator: NetworkActivityInitiator;
      readonly purpose: NetworkActivityPurpose;
      readonly receivedBytes: number;
      readonly sentBytes: number;
      readonly success: boolean;
    }>;
    readonly retentionDays: number;
    readonly totalCount: number;
    readonly totalReceivedBytes: number;
    readonly totalSentBytes: number;
  };
  readonly viewer: {
    readonly role: Role;
  };
};
export type networkActivityQuery = {
  response: networkActivityQuery$data;
  variables: networkActivityQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "role",
  "storageKey": null
},
v1 = {
  "alias": null,
  "args": null,
  "concreteType": "NetworkActivity",
  "kind": "LinkedField",
  "name": "networkActivity",
  "plural": false,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "enabled",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "enabledAt",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "retentionDays",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "totalCount",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "totalSentBytes",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "totalReceivedBytes",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "concreteType": "NetworkActivityEntry",
      "kind": "LinkedField",
      "name": "entries",
      "plural": true,
      "selections": [
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "hour",
          "storageKey": null
        },
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "purpose",
          "storageKey": null
        },
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "host",
          "storageKey": null
        },
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "initiator",
          "storageKey": null
        },
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "success",
          "storageKey": null
        },
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "count",
          "storageKey": null
        },
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "sentBytes",
          "storageKey": null
        },
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "receivedBytes",
          "storageKey": null
        }
      ],
      "storageKey": null
    }
  ],
  "storageKey": null
};
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "networkActivityQuery",
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "User",
        "kind": "LinkedField",
        "name": "viewer",
        "plural": false,
        "selections": [
          (v0/*:: as any*/)
        ],
        "storageKey": null
      },
      (v1/*:: as any*/)
    ],
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "networkActivityQuery",
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "User",
        "kind": "LinkedField",
        "name": "viewer",
        "plural": false,
        "selections": [
          (v0/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "id",
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      (v1/*:: as any*/)
    ]
  },
  "params": {
    "cacheID": "2126dd667a98857f5d2d54482b558ce7",
    "id": null,
    "metadata": {},
    "name": "networkActivityQuery",
    "operationKind": "query",
    "text": "query networkActivityQuery {\n  viewer {\n    role\n    id\n  }\n  networkActivity {\n    enabled\n    enabledAt\n    retentionDays\n    totalCount\n    totalSentBytes\n    totalReceivedBytes\n    entries {\n      hour\n      purpose\n      host\n      initiator\n      success\n      count\n      sentBytes\n      receivedBytes\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "7aacb164771eb047ac38667428deb93d";

export default node;
