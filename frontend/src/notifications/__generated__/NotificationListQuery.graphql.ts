/**
 * @generated SignedSource<<e963029048239aec964f86b71f005d9a>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type NotificationKind = "RECORDING_COMPLETED" | "RECORDING_FAILED" | "RECORDING_STARTED" | "RESERVATION_CONFLICT" | "STORAGE_LOW" | "TUNER_SERVER_UNREACHABLE" | "WATCH_REMINDER" | "%future added value";
export type NotificationListQuery$variables = {
  first: number;
};
export type NotificationListQuery$data = {
  readonly notifications: {
    readonly edges: ReadonlyArray<{
      readonly node: {
        readonly id: string;
        readonly kind: NotificationKind;
        readonly message: string;
        readonly occurredAt: string;
        readonly payload: string;
        readonly readAt: string | null | undefined;
        readonly url: string | null | undefined;
      };
    }>;
  };
  readonly unreadNotificationCount: number;
};
export type NotificationListQuery = {
  response: NotificationListQuery$data;
  variables: NotificationListQuery$variables;
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
                "name": "url",
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
    "name": "NotificationListQuery",
    "selections": (v1/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "NotificationListQuery",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "79103be40dc92aaa90a292865c8794c4",
    "id": null,
    "metadata": {},
    "name": "NotificationListQuery",
    "operationKind": "query",
    "text": "query NotificationListQuery(\n  $first: Int!\n) {\n  unreadNotificationCount\n  notifications(first: $first) {\n    edges {\n      node {\n        id\n        kind\n        occurredAt\n        message\n        payload\n        url\n        readAt\n      }\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "8e018e4d3202fe7e521662c3588a07cb";

export default node;
