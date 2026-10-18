/**
 * @generated SignedSource<<b7609ced7223760b0e92fe7acb99b031>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type remoteChangedSubscription$variables = Record<PropertyKey, never>;
export type remoteChangedSubscription$data = {
  readonly playersChanged: number;
};
export type remoteChangedSubscription = {
  response: remoteChangedSubscription$data;
  variables: remoteChangedSubscription$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "playersChanged",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "remoteChangedSubscription",
    "selections": (v0/*:: as any*/),
    "type": "Subscription",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "remoteChangedSubscription",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "1076777446071b6cc91fe57273a2a4c4",
    "id": null,
    "metadata": {},
    "name": "remoteChangedSubscription",
    "operationKind": "subscription",
    "text": "subscription remoteChangedSubscription {\n  playersChanged\n}\n"
  }
};
})();

(node as any).hash = "8c13421a4e15975951713d1937e874f1";

export default node;
