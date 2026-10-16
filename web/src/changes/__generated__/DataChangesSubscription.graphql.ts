/**
 * @generated SignedSource<<5b52bc6825017542fa8621df0d7a27cc>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type DataChannel = "ASSIGNMENTS" | "EPG" | "RESERVATIONS" | "RESERVATION_CONFLICTS" | "RULES" | "TUNERS" | "%future added value";
export type DataChangesSubscription$variables = Record<PropertyKey, never>;
export type DataChangesSubscription$data = {
  readonly dataChanged: {
    readonly channel: DataChannel;
    readonly version: number;
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
    "concreteType": "DataChange",
    "kind": "LinkedField",
    "name": "dataChanged",
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
    "cacheID": "78c1beba3ccd6260d6d7f64f637c076c",
    "id": null,
    "metadata": {},
    "name": "DataChangesSubscription",
    "operationKind": "subscription",
    "text": "subscription DataChangesSubscription {\n  dataChanged {\n    channel\n    version\n  }\n}\n"
  }
};
})();

(node as any).hash = "2cf0c4f10fd617131b8315ab05729c59";

export default node;
