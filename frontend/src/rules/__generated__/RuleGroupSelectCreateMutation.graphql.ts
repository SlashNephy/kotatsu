/**
 * @generated SignedSource<<5cc6eedba854d5ec04d1f1c6d323baf9>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type RuleGroupSelectCreateMutation$variables = {
  name: string;
};
export type RuleGroupSelectCreateMutation$data = {
  readonly createRuleGroup: {
    readonly id: string;
    readonly name: string;
    readonly position: number;
  };
};
export type RuleGroupSelectCreateMutation = {
  response: RuleGroupSelectCreateMutation$data;
  variables: RuleGroupSelectCreateMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
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
        "name": "name",
        "variableName": "name"
      }
    ],
    "concreteType": "RuleGroup",
    "kind": "LinkedField",
    "name": "createRuleGroup",
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
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "position",
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
    "name": "RuleGroupSelectCreateMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "RuleGroupSelectCreateMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "c3b2bc93a42cd10b6f03bf373d985d9b",
    "id": null,
    "metadata": {},
    "name": "RuleGroupSelectCreateMutation",
    "operationKind": "mutation",
    "text": "mutation RuleGroupSelectCreateMutation(\n  $name: String!\n) {\n  createRuleGroup(name: $name) {\n    id\n    name\n    position\n  }\n}\n"
  }
};
})();

(node as any).hash = "abc88d9b3b324cd09b7303f1a235dd89";

export default node;
