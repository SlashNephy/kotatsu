/**
 * @generated SignedSource<<a5218e518cd3cc620817c2a694e1b44d>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type NotificationKind = "RECORDING_COMPLETED" | "RECORDING_FAILED" | "RECORDING_STARTED" | "RESERVATION_CONFLICT" | "STORAGE_LOW" | "TUNER_SERVER_UNREACHABLE" | "%future added value";
export type notificationsListQuery$variables = {
  first: number;
};
export type notificationsListQuery$data = {
  readonly notifications: {
    readonly edges: ReadonlyArray<{
      readonly node: {
        readonly id: string;
        readonly kind: NotificationKind;
        readonly message: string;
        readonly occurredAt: string;
        readonly payload: string;
        readonly readAt: string | null | undefined;
      };
    }>;
  };
  readonly unreadNotificationCount: number;
};
export type notificationsListQuery = {
  response: notificationsListQuery$data;
  variables: notificationsListQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "first"
  }
],
v1 = [
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "unreadNotificationCount",
    "storageKey": null
  },
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "first",
        "variableName": "first"
      }
    ],
    "concreteType": "NotificationConnection",
    "kind": "LinkedField",
    "name": "notifications",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "NotificationEdge",
        "kind": "LinkedField",
        "name": "edges",
        "plural": true,
        "selections": [
          {
            "alias": null,
            "args": null,
            "concreteType": "Notification",
            "kind": "LinkedField",
            "name": "node",
            "plural": false,
            "selections": [
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "id",
                "storageKey": null
              },
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
                "name": "occurredAt",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "message",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "payload",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "readAt",
                "storageKey": null
              }
            ],
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
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "notificationsListQuery",
    "selections": (v1/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "notificationsListQuery",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "c4559c16a15973198099437881c8a5e6",
    "id": null,
    "metadata": {},
    "name": "notificationsListQuery",
    "operationKind": "query",
    "text": "query notificationsListQuery(\n  $first: Int!\n) {\n  unreadNotificationCount\n  notifications(first: $first) {\n    edges {\n      node {\n        id\n        kind\n        occurredAt\n        message\n        payload\n        readAt\n      }\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "9eba0c0a38bc2d5ea78bd862d441a239";

export default node;
