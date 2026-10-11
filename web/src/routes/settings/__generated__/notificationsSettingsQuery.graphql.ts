/**
 * @generated SignedSource<<bbb69944326a149aaffe9867e91784b2>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type NotificationKind = "RECORDING_COMPLETED" | "RECORDING_FAILED" | "RECORDING_STARTED" | "RESERVATION_CONFLICT" | "STORAGE_LOW" | "TUNER_SERVER_UNREACHABLE" | "%future added value";
export type notificationsSettingsQuery$variables = Record<PropertyKey, never>;
export type notificationsSettingsQuery$data = {
  readonly notificationPreferences: ReadonlyArray<{
    readonly inApp: boolean;
    readonly kind: NotificationKind;
    readonly webPush: boolean;
  }>;
  readonly webPushPublicKey: string;
  readonly webPushSubscriptions: ReadonlyArray<{
    readonly endpoint: string;
  }>;
};
export type notificationsSettingsQuery = {
  response: notificationsSettingsQuery$data;
  variables: notificationsSettingsQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "webPushPublicKey",
  "storageKey": null
},
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "endpoint",
  "storageKey": null
},
v2 = {
  "alias": null,
  "args": null,
  "concreteType": "NotificationPreference",
  "kind": "LinkedField",
  "name": "notificationPreferences",
  "plural": true,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "kind",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "inApp",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "webPush",
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
    "name": "notificationsSettingsQuery",
    "selections": [
      (v0/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "concreteType": "WebPushSubscription",
        "kind": "LinkedField",
        "name": "webPushSubscriptions",
        "plural": true,
        "selections": [
          (v1/*:: as any*/)
        ],
        "storageKey": null
      },
      (v2/*:: as any*/)
    ],
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "notificationsSettingsQuery",
    "selections": [
      (v0/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "concreteType": "WebPushSubscription",
        "kind": "LinkedField",
        "name": "webPushSubscriptions",
        "plural": true,
        "selections": [
          (v1/*:: as any*/),
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
      (v2/*:: as any*/)
    ]
  },
  "params": {
    "cacheID": "c9b2effd27485f5d51c6716216f8ab69",
    "id": null,
    "metadata": {},
    "name": "notificationsSettingsQuery",
    "operationKind": "query",
    "text": "query notificationsSettingsQuery {\n  webPushPublicKey\n  webPushSubscriptions {\n    endpoint\n    id\n  }\n  notificationPreferences {\n    kind\n    inApp\n    webPush\n  }\n}\n"
  }
};
})();

(node as any).hash = "f92ad9a471e877998dd09e49c9e649d4";

export default node;
