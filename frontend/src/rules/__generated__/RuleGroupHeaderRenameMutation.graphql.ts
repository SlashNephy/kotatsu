/**
 * @generated SignedSource<<a6f64a82cf935c165eeb2625d234a360>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type RuleGroupHeaderRenameMutation$variables = {
  id: string;
  name: string;
};
export type RuleGroupHeaderRenameMutation$data = {
  readonly renameRuleGroup: {
    readonly id: string;
    readonly name: string;
  };
};
export type RuleGroupHeaderRenameMutation = {
  response: RuleGroupHeaderRenameMutation$data;
  variables: RuleGroupHeaderRenameMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "id"
  },
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "name"
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
      },
      {
        "kind": "Variable",
        "name": "name",
        "variableName": "name"
      }
    ],
    "concreteType": "RuleGroup",
    "kind": "LinkedField",
    "name": "renameRuleGroup",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "id",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "name",
        "storageKey": null
      }
    ],
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "RuleGroupHeaderRenameMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "RuleGroupHeaderRenameMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "9da4a6b01183d4494bb337b62c101008",
    "id": null,
    "metadata": {},
    "name": "RuleGroupHeaderRenameMutation",
    "operationKind": "mutation",
    "text": "mutation RuleGroupHeaderRenameMutation(\n  $id: ID!\n  $name: String!\n) {\n  renameRuleGroup(id: $id, name: $name) {\n    id\n    name\n  }\n}\n"
  }
};
})();

(node as any).hash = "783dfcd263c31365c6ee4391f48daff4";

export default node;
