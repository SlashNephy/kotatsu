/**
 * @generated SignedSource<<95c24c3d6992201626ce0e5e0857316f>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type routesHeartbeatSubscription$variables = Record<PropertyKey, never>;
export type routesHeartbeatSubscription$data = {
  readonly heartbeat: string;
};
export type routesHeartbeatSubscription = {
  response: routesHeartbeatSubscription$data;
  variables: routesHeartbeatSubscription$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "heartbeat",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "routesHeartbeatSubscription",
    "selections": (v0/*:: as any*/),
    "type": "Subscription",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "routesHeartbeatSubscription",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "4cf6caf00c6896d0a1a5511bcb4f968e",
    "id": null,
    "metadata": {},
    "name": "routesHeartbeatSubscription",
    "operationKind": "subscription",
    "text": "subscription routesHeartbeatSubscription {\n  heartbeat\n}\n"
  }
};
})();

(node as any).hash = "0f141c3521ede3a34a83980b80014b64";

export default node;
