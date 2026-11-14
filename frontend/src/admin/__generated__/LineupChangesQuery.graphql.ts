/**
 * @generated SignedSource<<4e7e50ba0ffae429ec57524803515bd8>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type LineupChangesQuery$variables = Record<PropertyKey, never>;
export type LineupChangesQuery$data = {
  readonly notifications: {
    readonly edges: ReadonlyArray<{
      readonly node: {
        readonly id: string;
        readonly message: string;
        readonly occurredAt: string;
        readonly readAt: string | null | undefined;
      };
    }>;
  };
};
export type LineupChangesQuery = {
  response: LineupChangesQuery$data;
  variables: LineupChangesQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Literal",
        "name": "first",
        "value": 10
      },
      {
        "kind": "Literal",
        "name": "kinds",
        "value": [
          "LINEUP_CHANGED"
        ]
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
                "name": "message",
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
    "storageKey": "notifications(first:10,kinds:[\"LINEUP_CHANGED\"])"
  }
];
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "LineupChangesQuery",
    "selections": (v0/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "LineupChangesQuery",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "a7b68bc501e63dc24c419801bdc6a76d",
    "id": null,
    "metadata": {},
    "name": "LineupChangesQuery",
    "operationKind": "query",
    "text": "query LineupChangesQuery {\n  notifications(first: 10, kinds: [LINEUP_CHANGED]) {\n    edges {\n      node {\n        id\n        message\n        occurredAt\n        readAt\n      }\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "41dab4cc58dda060cceac30452c87434";

export default node;
