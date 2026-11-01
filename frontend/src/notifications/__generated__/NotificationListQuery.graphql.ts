/**
 * @generated SignedSource<<bcac8ee23f80a3fb4ea8a07326feaddf>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type NotificationKind = "ANIME_RECORDED" | "ANIME_RECORD_FAILED" | "RECORDING_COMPLETED" | "RECORDING_FAILED" | "RECORDING_STARTED" | "RESERVATION_CONFLICT" | "STORAGE_LOW" | "TUNER_SERVER_UNREACHABLE" | "WATCH_REMINDER" | "%future added value";
export type NotificationListQuery$variables = {
  first: number;
};
export type NotificationListQuery$data = {
  readonly notifications: {
    readonly edges: ReadonlyArray<{
      readonly node: {
        readonly externalUrl: string | null | undefined;
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
                "name": "externalUrl",
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
    "cacheID": "3e39dbcbba9f1d4b26c1cad36cfa60ed",
    "id": null,
    "metadata": {},
    "name": "NotificationListQuery",
    "operationKind": "query",
    "text": "query NotificationListQuery(\n  $first: Int!\n) {\n  unreadNotificationCount\n  notifications(first: $first) {\n    edges {\n      node {\n        id\n        kind\n        occurredAt\n        message\n        payload\n        url\n        externalUrl\n        readAt\n      }\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "21fa5f6b662e31d0fc4b4834b94c4c24";

export default node;
