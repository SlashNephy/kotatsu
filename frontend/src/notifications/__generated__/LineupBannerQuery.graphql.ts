/**
 * @generated SignedSource<<a6ecc2010d4fdc197f71f830beb4405f>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type LineupBannerQuery$variables = Record<PropertyKey, never>;
export type LineupBannerQuery$data = {
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
export type LineupBannerQuery = {
  response: LineupBannerQuery$data;
  variables: LineupBannerQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Literal",
        "name": "first",
        "value": 5
      },
      {
        "kind": "Literal",
        "name": "kinds",
        "value": [
          "LINEUP_CHANGED"
        ]
      },
      {
        "kind": "Literal",
        "name": "unreadOnly",
        "value": true
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
    "storageKey": "notifications(first:5,kinds:[\"LINEUP_CHANGED\"],unreadOnly:true)"
  }
];
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "LineupBannerQuery",
    "selections": (v0/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "LineupBannerQuery",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "9d564e7b731703f57f03620f8d53599e",
    "id": null,
    "metadata": {},
    "name": "LineupBannerQuery",
    "operationKind": "query",
    "text": "query LineupBannerQuery {\n  notifications(first: 5, unreadOnly: true, kinds: [LINEUP_CHANGED]) {\n    edges {\n      node {\n        id\n        message\n        occurredAt\n        readAt\n      }\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "5130930255c47eb6838259d121cb2dc7";

export default node;
