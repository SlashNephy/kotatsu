/**
 * @generated SignedSource<<dfa4b19020a213d58f92d2349e5bbc7c>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type notificationsMarkMutation$variables = {
  ids: ReadonlyArray<string>;
};
export type notificationsMarkMutation$data = {
  readonly markNotificationsRead: number;
};
export type notificationsMarkMutation = {
  response: notificationsMarkMutation$data;
  variables: notificationsMarkMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "ids"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "ids",
        "variableName": "ids"
      }
    ],
    "kind": "ScalarField",
    "name": "markNotificationsRead",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "notificationsMarkMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "notificationsMarkMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "ab67e6481a51f666cbc3486fab5f316e",
    "id": null,
    "metadata": {},
    "name": "notificationsMarkMutation",
    "operationKind": "mutation",
    "text": "mutation notificationsMarkMutation(\n  $ids: [ID!]!\n) {\n  markNotificationsRead(ids: $ids)\n}\n"
  }
};
})();

(node as any).hash = "c510af2d8f7b51b4dcdd6f5c038b3d4a";

export default node;
