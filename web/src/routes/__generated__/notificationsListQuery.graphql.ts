/**
 * @generated SignedSource<<94066af9d48f2228e3ffa016035b0cc4>>
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
        readonly readAt: string | null | undefined;
      };
    }>;
  };
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
    "cacheID": "aef59641af446f20a04791a7a8333973",
    "id": null,
    "metadata": {},
    "name": "notificationsListQuery",
    "operationKind": "query",
    "text": "query notificationsListQuery(\n  $first: Int!\n) {\n  notifications(first: $first) {\n    edges {\n      node {\n        id\n        kind\n        occurredAt\n        message\n        readAt\n      }\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "ac94e6bd08633ccd3f6737504171ddca";

export default node;
