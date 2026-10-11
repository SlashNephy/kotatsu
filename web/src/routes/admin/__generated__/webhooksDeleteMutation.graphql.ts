/**
 * @generated SignedSource<<133ce199adc6a0b81f55863a75dac22b>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type webhooksDeleteMutation$variables = {
  id: string;
};
export type webhooksDeleteMutation$data = {
  readonly deleteWebhook: string;
};
export type webhooksDeleteMutation = {
  response: webhooksDeleteMutation$data;
  variables: webhooksDeleteMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "id"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "id",
        "variableName": "id"
      }
    ],
    "kind": "ScalarField",
    "name": "deleteWebhook",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "webhooksDeleteMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "webhooksDeleteMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "9646aa73a38c5d0f1edc04d3fa104907",
    "id": null,
    "metadata": {},
    "name": "webhooksDeleteMutation",
    "operationKind": "mutation",
    "text": "mutation webhooksDeleteMutation(\n  $id: ID!\n) {\n  deleteWebhook(id: $id)\n}\n"
  }
};
})();

(node as any).hash = "8e933b744288a94383c73e798eb490d2";

export default node;
