/**
 * @generated SignedSource<<0b09ee7e7e01791dbeb25f45555b895a>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type notificationsMarkAllMutation$variables = Record<PropertyKey, never>;
export type notificationsMarkAllMutation$data = {
  readonly markAllNotificationsRead: number;
};
export type notificationsMarkAllMutation = {
  response: notificationsMarkAllMutation$data;
  variables: notificationsMarkAllMutation$variables;
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
    "name": "notificationsMarkAllMutation",
    "selections": (v0/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "notificationsMarkAllMutation",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "729a9ff913ce2059524d3e53cdd2b9c4",
    "id": null,
    "metadata": {},
    "name": "notificationsMarkAllMutation",
    "operationKind": "mutation",
    "text": "mutation notificationsMarkAllMutation {\n  markAllNotificationsRead\n}\n"
  }
};
})();

(node as any).hash = "214d7cb63319f8ebe53eed2ef5c35a4c";

export default node;
