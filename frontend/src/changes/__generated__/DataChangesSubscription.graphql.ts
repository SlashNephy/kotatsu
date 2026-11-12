/**
 * @generated SignedSource<<a03435aca5c8ff950ad2cc4eeda735aa>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type DataChannel = "ADMIN" | "ANIME_FOLLOWING" | "ASSIGNMENTS" | "EPG" | "RECORDINGS" | "RESERVATIONS" | "RESERVATION_CONFLICTS" | "RULES" | "TUNERS" | "%future added value";
export type DataChangesSubscription$variables = Record<PropertyKey, never>;
export type DataChangesSubscription$data = {
  readonly viewerEvents: {
    readonly dataChange: {
      readonly channel: DataChannel;
      readonly version: number;
    } | null | undefined;
    readonly liveStreamScrambled: {
      readonly decidedAt: string;
      readonly service: string;
    } | null | undefined;
    readonly streamsDraining: boolean | null | undefined;
    readonly unreadNotificationCount: number | null | undefined;
    readonly viewerSettingsVersion: number | null | undefined;
  };
};
export type DataChangesSubscription = {
  response: DataChangesSubscription$data;
  variables: DataChangesSubscription$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "ViewerEvent",
    "kind": "LinkedField",
    "name": "viewerEvents",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "DataChange",
        "kind": "LinkedField",
        "name": "dataChange",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "channel",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "version",
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "viewerSettingsVersion",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "unreadNotificationCount",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "streamsDraining",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "LiveStreamScrambled",
        "kind": "LinkedField",
        "name": "liveStreamScrambled",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "service",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "decidedAt",
            "storageKey": null
          }
        ],
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
    "name": "DataChangesSubscription",
    "selections": (v0/*:: as any*/),
    "type": "Subscription",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "DataChangesSubscription",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "8a8da5ed8e5d47185c70aefb0541a728",
    "id": null,
    "metadata": {},
    "name": "DataChangesSubscription",
    "operationKind": "subscription",
    "text": "subscription DataChangesSubscription {\n  viewerEvents {\n    dataChange {\n      channel\n      version\n    }\n    viewerSettingsVersion\n    unreadNotificationCount\n    streamsDraining\n    liveStreamScrambled {\n      service\n      decidedAt\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "4228e2725fddd985b91f08d4f7d40ae4";

export default node;
