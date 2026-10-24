/**
 * @generated SignedSource<<05d0090a762e4b318b82280f3e5ebb4c>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type RuleEditorDeleteMutation$variables = {
  id: string;
};
export type RuleEditorDeleteMutation$data = {
  readonly deleteRule: string;
};
export type RuleEditorDeleteMutation = {
  response: RuleEditorDeleteMutation$data;
  variables: RuleEditorDeleteMutation$variables;
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
    "name": "deleteRule",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "RuleEditorDeleteMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "RuleEditorDeleteMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "a1f81c244c72e337d9fb16501f460926",
    "id": null,
    "metadata": {},
    "name": "RuleEditorDeleteMutation",
    "operationKind": "mutation",
    "text": "mutation RuleEditorDeleteMutation(\n  $id: ID!\n) {\n  deleteRule(id: $id)\n}\n"
  }
};
})();

(node as any).hash = "bedf31c97960330485e1771706bb2191";

export default node;
