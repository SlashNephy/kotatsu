/**
 * @generated SignedSource<<1b91deadcc4ccf19643ee74cc3a04af3>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type DataChannel = "ADMIN" | "ASSIGNMENTS" | "EPG" | "RECORDINGS" | "RESERVATIONS" | "RESERVATION_CONFLICTS" | "RULES" | "TUNERS" | "%future added value";
export type DataChangesSubscription$variables = Record<PropertyKey, never>;
export type DataChangesSubscription$data = {
  readonly viewerEvents: {
    readonly dataChange: {
      readonly channel: DataChannel;
      readonly version: number;
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
    "cacheID": "2740803792ebfc48b6d29e442e5ab728",
    "id": null,
    "metadata": {},
    "name": "DataChangesSubscription",
    "operationKind": "subscription",
    "text": "subscription DataChangesSubscription {\n  viewerEvents {\n    dataChange {\n      channel\n      version\n    }\n    viewerSettingsVersion\n    unreadNotificationCount\n    streamsDraining\n  }\n}\n"
  }
};
})();

(node as any).hash = "84fb8348c394cd96f949cefb7ef0412d";

export default node;
