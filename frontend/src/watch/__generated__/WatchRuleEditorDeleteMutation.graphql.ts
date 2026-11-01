/**
 * @generated SignedSource<<3b41fd8e13303d267c7425f25eacd414>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type WatchRuleEditorDeleteMutation$variables = {
  id: string;
};
export type WatchRuleEditorDeleteMutation$data = {
  readonly deleteWatchRule: string;
};
export type WatchRuleEditorDeleteMutation = {
  response: WatchRuleEditorDeleteMutation$data;
  variables: WatchRuleEditorDeleteMutation$variables;
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
    "name": "deleteWatchRule",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "WatchRuleEditorDeleteMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "WatchRuleEditorDeleteMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "9bdc4886681a71106b717939a683a34a",
    "id": null,
    "metadata": {},
    "name": "WatchRuleEditorDeleteMutation",
    "operationKind": "mutation",
    "text": "mutation WatchRuleEditorDeleteMutation(\n  $id: ID!\n) {\n  deleteWatchRule(id: $id)\n}\n"
  }
};
})();

(node as any).hash = "94ab0ad31abdc306e1f7cd8513de87ea";

export default node;
