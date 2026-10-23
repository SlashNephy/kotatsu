/**
 * @generated SignedSource<<c83252974c28bf39d9567cc99f7d6ee9>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type WebPushSubscriptionInput = {
  auth: string;
  endpoint: string;
  p256dh: string;
};
export type notificationsSubscribeMutation$variables = {
  input: WebPushSubscriptionInput;
};
export type notificationsSubscribeMutation$data = {
  readonly subscribeWebPush: {
    readonly endpoint: string;
  };
};
export type notificationsSubscribeMutation = {
  response: notificationsSubscribeMutation$data;
  variables: notificationsSubscribeMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "input"
  }
],
v1 = [
  {
    "kind": "Variable",
    "name": "input",
    "variableName": "input"
  }
],
v2 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "endpoint",
  "storageKey": null
};
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "notificationsSubscribeMutation",
    "selections": [
      {
        "alias": null,
        "args": (v1/*:: as any*/),
        "concreteType": "WebPushSubscription",
        "kind": "LinkedField",
        "name": "subscribeWebPush",
        "plural": false,
        "selections": [
          (v2/*:: as any*/)
        ],
        "storageKey": null
      }
    ],
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "notificationsSubscribeMutation",
    "selections": [
      {
        "alias": null,
        "args": (v1/*:: as any*/),
        "concreteType": "WebPushSubscription",
        "kind": "LinkedField",
        "name": "subscribeWebPush",
        "plural": false,
        "selections": [
          (v2/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "id",
            "storageKey": null
          }
        ],
        "storageKey": null
      }
    ]
  },
  "params": {
    "cacheID": "1f8bbc37e26d027d98bb66cefb0cc1fb",
    "id": null,
    "metadata": {},
    "name": "notificationsSubscribeMutation",
    "operationKind": "mutation",
    "text": "mutation notificationsSubscribeMutation(\n  $input: WebPushSubscriptionInput!\n) {\n  subscribeWebPush(input: $input) {\n    endpoint\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "545d908b6799fe99735c89b3662a99dd";

export default node;
