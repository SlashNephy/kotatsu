/**
 * @generated SignedSource<<98dd8bf6fe1d1effaf2483127388de81>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type NotificationKind = "ANIME_RECORDED" | "ANIME_RECORD_FAILED" | "RECORDING_COMPLETED" | "RECORDING_FAILED" | "RECORDING_STARTED" | "RESERVATION_CONFLICT" | "SCRAMBLED_RECORDINGS" | "STORAGE_LOW" | "TUNER_SERVER_UNREACHABLE" | "WATCH_REMINDER" | "%future added value";
export type Role = "ADMIN" | "MEMBER" | "VIEWER" | "%future added value";
export type notificationsSettingsQuery$variables = Record<PropertyKey, never>;
export type notificationsSettingsQuery$data = {
  readonly notificationPreferences: ReadonlyArray<{
    readonly inApp: boolean;
    readonly kind: NotificationKind;
    readonly ruleReservations: boolean;
    readonly webPush: boolean;
  }>;
  readonly viewer: {
    readonly role: Role;
  };
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
  "name": "role",
  "storageKey": null
},
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "webPushPublicKey",
  "storageKey": null
},
v2 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "endpoint",
  "storageKey": null
},
v3 = {
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
v4 = {
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
},
v5 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
};
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "notificationsSettingsQuery",
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
      (v1/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "concreteType": "WebPushSubscription",
        "kind": "LinkedField",
        "name": "webPushSubscriptions",
        "plural": true,
        "selections": [
          (v2/*:: as any*/)
        ],
        "storageKey": null
      },
      (v3/*:: as any*/),
      (v4/*:: as any*/)
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
      {
        "alias": null,
        "args": null,
        "concreteType": "User",
        "kind": "LinkedField",
        "name": "viewer",
        "plural": false,
        "selections": [
          (v0/*:: as any*/),
          (v5/*:: as any*/)
        ],
        "storageKey": null
      },
      (v1/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "concreteType": "WebPushSubscription",
        "kind": "LinkedField",
        "name": "webPushSubscriptions",
        "plural": true,
        "selections": [
          (v2/*:: as any*/),
          (v5/*:: as any*/)
        ],
        "storageKey": null
      },
      (v3/*:: as any*/),
      (v4/*:: as any*/)
    ]
  },
  "params": {
    "cacheID": "d21774baa8a3b3f2d97a135be99ae44e",
    "id": null,
    "metadata": {},
    "name": "notificationsSettingsQuery",
    "operationKind": "query",
    "text": "query notificationsSettingsQuery {\n  viewer {\n    role\n    id\n  }\n  webPushPublicKey\n  webPushSubscriptions {\n    endpoint\n    id\n  }\n  notificationPreferences {\n    kind\n    inApp\n    webPush\n    ruleReservations\n  }\n  watchReminderSettings {\n    minutesBefore\n  }\n}\n"
  }
};
})();

(node as any).hash = "072f35da0bfd502944a2e64795179698";

export default node;
