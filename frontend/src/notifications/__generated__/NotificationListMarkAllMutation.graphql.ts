/**
 * @generated SignedSource<<519bb6c4ec5883f87ae2d66df024f052>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type NotificationListMarkAllMutation$variables = Record<PropertyKey, never>;
export type NotificationListMarkAllMutation$data = {
  readonly markAllNotificationsRead: number;
};
export type NotificationListMarkAllMutation = {
  response: NotificationListMarkAllMutation$data;
  variables: NotificationListMarkAllMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "markAllNotificationsRead",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "NotificationListMarkAllMutation",
    "selections": (v0/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "NotificationListMarkAllMutation",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "889978789ae4a30378230382583d1089",
    "id": null,
    "metadata": {},
    "name": "NotificationListMarkAllMutation",
    "operationKind": "mutation",
    "text": "mutation NotificationListMarkAllMutation {\n  markAllNotificationsRead\n}\n"
  }
};
})();

(node as any).hash = "0e2bdba46c3b0f67f9d1204faa71f312";

export default node;
