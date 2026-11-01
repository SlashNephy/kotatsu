/**
 * @generated SignedSource<<5c3e25cef22e65ffbc4256571df98489>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type capturesDeleteMutation$variables = {
  id: string;
};
export type capturesDeleteMutation$data = {
  readonly deleteCapture: string;
};
export type capturesDeleteMutation = {
  response: capturesDeleteMutation$data;
  variables: capturesDeleteMutation$variables;
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
    "name": "deleteCapture",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "capturesDeleteMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "capturesDeleteMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "459d7c47a164ebb2f8532a79fbed61f0",
    "id": null,
    "metadata": {},
    "name": "capturesDeleteMutation",
    "operationKind": "mutation",
    "text": "mutation capturesDeleteMutation(\n  $id: ID!\n) {\n  deleteCapture(id: $id)\n}\n"
  }
};
})();

(node as any).hash = "fa12bf99bd3b3c36b2a6118f4e5d70e4";

export default node;
