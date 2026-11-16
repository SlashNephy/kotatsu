/**
 * @generated SignedSource<<3da89c278cdaca787a20852545c106c6>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type RuleGroupHeaderDeleteMutation$variables = {
  id: string;
};
export type RuleGroupHeaderDeleteMutation$data = {
  readonly deleteRuleGroup: string;
};
export type RuleGroupHeaderDeleteMutation = {
  response: RuleGroupHeaderDeleteMutation$data;
  variables: RuleGroupHeaderDeleteMutation$variables;
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
    "name": "deleteRuleGroup",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "RuleGroupHeaderDeleteMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "RuleGroupHeaderDeleteMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "b49473edf8700e0d83d233e0e84e819f",
    "id": null,
    "metadata": {},
    "name": "RuleGroupHeaderDeleteMutation",
    "operationKind": "mutation",
    "text": "mutation RuleGroupHeaderDeleteMutation(\n  $id: ID!\n) {\n  deleteRuleGroup(id: $id)\n}\n"
  }
};
})();

(node as any).hash = "414a14589a0fb9773c2bcc0afeaade00";

export default node;
