/**
 * @generated SignedSource<<7daa7c3a60159ac5a8e1c43944f8be2d>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type integrationsRevokeStreamUrlMutation$variables = {
  id: string;
};
export type integrationsRevokeStreamUrlMutation$data = {
  readonly revokeStreamUrl: boolean;
};
export type integrationsRevokeStreamUrlMutation = {
  response: integrationsRevokeStreamUrlMutation$data;
  variables: integrationsRevokeStreamUrlMutation$variables;
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
    "name": "revokeStreamUrl",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "integrationsRevokeStreamUrlMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "integrationsRevokeStreamUrlMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "45ecf578b3165ffbeb20c69f4c6d609c",
    "id": null,
    "metadata": {},
    "name": "integrationsRevokeStreamUrlMutation",
    "operationKind": "mutation",
    "text": "mutation integrationsRevokeStreamUrlMutation(\n  $id: ID!\n) {\n  revokeStreamUrl(id: $id)\n}\n"
  }
};
})();

(node as any).hash = "72636731ac8a5b12e2cd216f03f18b7f";

export default node;
