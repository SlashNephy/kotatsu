/**
 * @generated SignedSource<<ad3e2d92cf6477d03ab9a82e6f6a83ac>>
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
    "cacheID": "941a42f26f772caeb84857be43cba5fd",
    "id": null,
    "metadata": {},
    "name": "LineupBannerQuery",
    "operationKind": "query",
    "text": "query LineupBannerQuery {\n  notifications(first: 5, unreadOnly: true, kinds: [LINEUP_CHANGED]) {\n    edges {\n      node {\n        id\n        message\n        occurredAt\n      }\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "3cb6ccba0575b0f9a205a9ecfad9ca87";

export default node;
