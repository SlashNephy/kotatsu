/**
 * @generated SignedSource<<5ca0876c4c7133eb7c1e2c59f3a4717f>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type UserMenuUnsubscribeMutation$variables = {
  endpoint: string;
};
export type UserMenuUnsubscribeMutation$data = {
  readonly unsubscribeWebPush: boolean;
};
export type UserMenuUnsubscribeMutation = {
  response: UserMenuUnsubscribeMutation$data;
  variables: UserMenuUnsubscribeMutation$variables;
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
    "name": "UserMenuUnsubscribeMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "UserMenuUnsubscribeMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "da955da9750abeb775222eceee6f3757",
    "id": null,
    "metadata": {},
    "name": "UserMenuUnsubscribeMutation",
    "operationKind": "mutation",
    "text": "mutation UserMenuUnsubscribeMutation(\n  $endpoint: String!\n) {\n  unsubscribeWebPush(endpoint: $endpoint)\n}\n"
  }
};
})();

(node as any).hash = "4ec0900b6680134050864ce7717c849d";

export default node;
