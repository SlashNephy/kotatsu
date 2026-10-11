/**
 * @generated SignedSource<<ab7c90bb320360f4aa94110317d004c5>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type NotificationBellSubscription$variables = Record<PropertyKey, never>;
export type NotificationBellSubscription$data = {
  readonly unreadNotificationCount: number;
};
export type NotificationBellSubscription = {
  response: NotificationBellSubscription$data;
  variables: NotificationBellSubscription$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "unreadNotificationCount",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "NotificationBellSubscription",
    "selections": (v0/*:: as any*/),
    "type": "Subscription",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "NotificationBellSubscription",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "98741aaf7f6fc5587a1c49c0b2baf54b",
    "id": null,
    "metadata": {},
    "name": "NotificationBellSubscription",
    "operationKind": "subscription",
    "text": "subscription NotificationBellSubscription {\n  unreadNotificationCount\n}\n"
  }
};
})();

(node as any).hash = "55f2b468c9da9b4e57d5bf0f1135b3ec";

export default node;
