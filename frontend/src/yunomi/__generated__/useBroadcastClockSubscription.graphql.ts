/**
 * @generated SignedSource<<7604498350a69558bcb2a07d207d1ca1>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type useBroadcastClockSubscription$variables = {
  service: string;
};
export type useBroadcastClockSubscription$data = {
  readonly liveStreamRates: {
    readonly broadcastClock: {
      readonly pcr: number;
      readonly receivedAt: string;
    } | null | undefined;
    readonly serverTime: string;
  };
};
export type useBroadcastClockSubscription = {
  response: useBroadcastClockSubscription$data;
  variables: useBroadcastClockSubscription$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "service"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "service",
        "variableName": "service"
      }
    ],
    "concreteType": "LiveStreamRates",
    "kind": "LinkedField",
    "name": "liveStreamRates",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "BroadcastClock",
        "kind": "LinkedField",
        "name": "broadcastClock",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "pcr",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "receivedAt",
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "serverTime",
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
    "name": "useBroadcastClockSubscription",
    "selections": (v1/*:: as any*/),
    "type": "Subscription",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "useBroadcastClockSubscription",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "06c456e1df189a53da363cbb13d50b59",
    "id": null,
    "metadata": {},
    "name": "useBroadcastClockSubscription",
    "operationKind": "subscription",
    "text": "subscription useBroadcastClockSubscription(\n  $service: ID!\n) {\n  liveStreamRates(service: $service) {\n    broadcastClock {\n      pcr\n      receivedAt\n    }\n    serverTime\n  }\n}\n"
  }
};
})();

(node as any).hash = "8edb3c95a61f435e4343f0b88cec3200";

export default node;
