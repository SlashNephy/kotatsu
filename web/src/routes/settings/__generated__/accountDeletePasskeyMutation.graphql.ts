/**
 * @generated SignedSource<<6a57766f32fad130f0ce0f732277ab40>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type accountDeletePasskeyMutation$variables = {
  id: string;
};
export type accountDeletePasskeyMutation$data = {
  readonly deletePasskey: string;
};
export type accountDeletePasskeyMutation = {
  response: accountDeletePasskeyMutation$data;
  variables: accountDeletePasskeyMutation$variables;
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
    "name": "deletePasskey",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "accountDeletePasskeyMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "accountDeletePasskeyMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "9fb34aef25b0900b8bcb242f8bd62eac",
    "id": null,
    "metadata": {},
    "name": "accountDeletePasskeyMutation",
    "operationKind": "mutation",
    "text": "mutation accountDeletePasskeyMutation(\n  $id: ID!\n) {\n  deletePasskey(id: $id)\n}\n"
  }
};
})();

(node as any).hash = "aa9a7e88272d1dfe58d9e62dd360fcb8";

export default node;
