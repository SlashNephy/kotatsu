/**
 * @generated SignedSource<<1afcad3b3979fa87a45c894dec241be9>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type DeleteAccountMutation$variables = Record<PropertyKey, never>;
export type DeleteAccountMutation$data = {
  readonly deleteAccount: boolean;
};
export type DeleteAccountMutation = {
  response: DeleteAccountMutation$data;
  variables: DeleteAccountMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "deleteAccount",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "DeleteAccountMutation",
    "selections": (v0/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "DeleteAccountMutation",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "18a6ca5311770e46e7a9c42bcb8ee5ab",
    "id": null,
    "metadata": {},
    "name": "DeleteAccountMutation",
    "operationKind": "mutation",
    "text": "mutation DeleteAccountMutation {\n  deleteAccount\n}\n"
  }
};
})();

(node as any).hash = "6dfdbc1a8b2d7d39f0a751d32ccbd4ac";

export default node;
