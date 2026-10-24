/**
 * @generated SignedSource<<7cf63da13bac9815931f42093bf08afe>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type notificationsUnsubscribeMutation$variables = {
  endpoint: string;
};
export type notificationsUnsubscribeMutation$data = {
  readonly unsubscribeWebPush: boolean;
};
export type notificationsUnsubscribeMutation = {
  response: notificationsUnsubscribeMutation$data;
  variables: notificationsUnsubscribeMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "endpoint"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "endpoint",
        "variableName": "endpoint"
      }
    ],
    "kind": "ScalarField",
    "name": "unsubscribeWebPush",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "notificationsUnsubscribeMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "notificationsUnsubscribeMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "58e480f086c8de37beb06a7ec7a83af0",
    "id": null,
    "metadata": {},
    "name": "notificationsUnsubscribeMutation",
    "operationKind": "mutation",
    "text": "mutation notificationsUnsubscribeMutation(\n  $endpoint: String!\n) {\n  unsubscribeWebPush(endpoint: $endpoint)\n}\n"
  }
};
})();

(node as any).hash = "92dee5c94b50cbbdc5b9bd282b259292";

export default node;
