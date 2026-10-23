/**
 * @generated SignedSource<<2fd6d8cb6f106d481159dc57b101833d>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type sessionsRevokeMutation$variables = {
  id: string;
};
export type sessionsRevokeMutation$data = {
  readonly revokeSession: string;
};
export type sessionsRevokeMutation = {
  response: sessionsRevokeMutation$data;
  variables: sessionsRevokeMutation$variables;
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
    "name": "revokeSession",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "sessionsRevokeMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "sessionsRevokeMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "763b9dcd1bb1d59c0d898c8647e9d212",
    "id": null,
    "metadata": {},
    "name": "sessionsRevokeMutation",
    "operationKind": "mutation",
    "text": "mutation sessionsRevokeMutation(\n  $id: ID!\n) {\n  revokeSession(id: $id)\n}\n"
  }
};
})();

(node as any).hash = "313f9e53e12b589908b8ce8b08ae48fd";

export default node;
