/**
 * @generated SignedSource<<eec94db3405c1359ce9111ce8ac20d38>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type NotificationKind = "ANIME_RECORDED" | "ANIME_RECORD_FAILED" | "RECORDING_COMPLETED" | "RECORDING_FAILED" | "RECORDING_STARTED" | "RESERVATION_CONFLICT" | "STORAGE_LOW" | "TUNER_SERVER_UNREACHABLE" | "WATCH_REMINDER" | "%future added value";
export type notificationsSettingsQuery$variables = Record<PropertyKey, never>;
export type notificationsSettingsQuery$data = {
  readonly notificationPreferences: ReadonlyArray<{
    readonly inApp: boolean;
    readonly kind: NotificationKind;
    readonly ruleReservations: boolean;
    readonly webPush: boolean;
  }>;
  readonly watchReminderSettings: {
    readonly minutesBefore: number;
  };
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
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "ruleReservations",
      "storageKey": null
    }
  ],
  "storageKey": null
},
v3 = {
  "alias": null,
  "args": null,
  "concreteType": "WatchReminderSettings",
  "kind": "LinkedField",
  "name": "watchReminderSettings",
  "plural": false,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "minutesBefore",
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
      (v2/*:: as any*/),
      (v3/*:: as any*/)
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
      (v2/*:: as any*/),
      (v3/*:: as any*/)
    ]
  },
  "params": {
    "cacheID": "e33b4c0946520be0075ce6de8aea66fd",
    "id": null,
    "metadata": {},
    "name": "notificationsSettingsQuery",
    "operationKind": "query",
    "text": "query notificationsSettingsQuery {\n  webPushPublicKey\n  webPushSubscriptions {\n    endpoint\n    id\n  }\n  notificationPreferences {\n    kind\n    inApp\n    webPush\n    ruleReservations\n  }\n  watchReminderSettings {\n    minutesBefore\n  }\n}\n"
  }
};
})();

(node as any).hash = "3d5f9561c9bc73b318c938e73de085be";

export default node;
