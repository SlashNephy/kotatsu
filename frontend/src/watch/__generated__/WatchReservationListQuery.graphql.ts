/**
 * @generated SignedSource<<c79330e1c03bc9f08b21901254d43f63>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type WatchReservationListQuery$variables = Record<PropertyKey, never>;
export type WatchReservationListQuery$data = {
  readonly watchReservations: ReadonlyArray<{
    readonly notifyAt: string;
    readonly program: {
      readonly endAt: string;
      readonly id: string;
      readonly name: string;
      readonly service: {
        readonly channelType: {
          readonly type: string;
        };
        readonly logoUrl: string | null | undefined;
        readonly name: string;
      };
      readonly startAt: string;
      readonly watchReserved: boolean;
    };
  }>;
};
export type WatchReservationListQuery = {
  response: WatchReservationListQuery$data;
  variables: WatchReservationListQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "notifyAt",
  "storageKey": null
},
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v2 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v3 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "startAt",
  "storageKey": null
},
v4 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "endAt",
  "storageKey": null
},
v5 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "watchReserved",
  "storageKey": null
},
v6 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "logoUrl",
  "storageKey": null
},
v7 = {
  "alias": null,
  "args": null,
  "concreteType": "ChannelType",
  "kind": "LinkedField",
  "name": "channelType",
  "plural": false,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "type",
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
    "name": "WatchReservationListQuery",
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "WatchReservation",
        "kind": "LinkedField",
        "name": "watchReservations",
        "plural": true,
        "selections": [
          (v0/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "concreteType": "Program",
            "kind": "LinkedField",
            "name": "program",
            "plural": false,
            "selections": [
              (v1/*:: as any*/),
              (v2/*:: as any*/),
              (v3/*:: as any*/),
              (v4/*:: as any*/),
              (v5/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "Service",
                "kind": "LinkedField",
                "name": "service",
                "plural": false,
                "selections": [
                  (v2/*:: as any*/),
                  (v6/*:: as any*/),
                  (v7/*:: as any*/)
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          }
        ],
        "storageKey": null
      }
    ],
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "WatchReservationListQuery",
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "WatchReservation",
        "kind": "LinkedField",
        "name": "watchReservations",
        "plural": true,
        "selections": [
          (v0/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "concreteType": "Program",
            "kind": "LinkedField",
            "name": "program",
            "plural": false,
            "selections": [
              (v1/*:: as any*/),
              (v2/*:: as any*/),
              (v3/*:: as any*/),
              (v4/*:: as any*/),
              (v5/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "Service",
                "kind": "LinkedField",
                "name": "service",
                "plural": false,
                "selections": [
                  (v2/*:: as any*/),
                  (v6/*:: as any*/),
                  (v7/*:: as any*/),
                  (v1/*:: as any*/)
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          }
        ],
        "storageKey": null
      }
    ]
  },
  "params": {
    "cacheID": "471b88dc8d0fd53c4e3a799e9f622647",
    "id": null,
    "metadata": {},
    "name": "WatchReservationListQuery",
    "operationKind": "query",
    "text": "query WatchReservationListQuery {\n  watchReservations {\n    notifyAt\n    program {\n      id\n      name\n      startAt\n      endAt\n      watchReserved\n      service {\n        name\n        logoUrl\n        channelType {\n          type\n        }\n        id\n      }\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "5cecb7affa8055712b9917e88d8dddf7";

export default node;
