/**
 * @generated SignedSource<<d1249941844785877ac2e27138b28c72>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type integrationsWebhookDeleteMutation$variables = {
  id: string;
};
export type integrationsWebhookDeleteMutation$data = {
  readonly deleteWebhook: string;
};
export type integrationsWebhookDeleteMutation = {
  response: integrationsWebhookDeleteMutation$data;
  variables: integrationsWebhookDeleteMutation$variables;
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
    "name": "integrationsWebhookDeleteMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "integrationsWebhookDeleteMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "d31df4eb6eae23295a4f194bdc9511c5",
    "id": null,
    "metadata": {},
    "name": "integrationsWebhookDeleteMutation",
    "operationKind": "mutation",
    "text": "mutation integrationsWebhookDeleteMutation(\n  $id: ID!\n) {\n  deleteWebhook(id: $id)\n}\n"
  }
};
})();

(node as any).hash = "dc19dc6d1af58fa9a0179e7ebe9da93e";

export default node;
