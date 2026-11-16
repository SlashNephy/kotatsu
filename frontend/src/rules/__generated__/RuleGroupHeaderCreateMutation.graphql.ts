/**
 * @generated SignedSource<<4c798ddee95fa4fc76cbcf3057c13ca6>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type RuleGroupHeaderCreateMutation$variables = {
  name: string;
};
export type RuleGroupHeaderCreateMutation$data = {
  readonly createRuleGroup: {
    readonly id: string;
    readonly name: string;
    readonly position: number;
  };
};
export type RuleGroupHeaderCreateMutation = {
  response: RuleGroupHeaderCreateMutation$data;
  variables: RuleGroupHeaderCreateMutation$variables;
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
    "name": "RuleGroupHeaderCreateMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "RuleGroupHeaderCreateMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "8cc6732ca277bb6b6a2359adb45ac59c",
    "id": null,
    "metadata": {},
    "name": "RuleGroupHeaderCreateMutation",
    "operationKind": "mutation",
    "text": "mutation RuleGroupHeaderCreateMutation(\n  $name: String!\n) {\n  createRuleGroup(name: $name) {\n    id\n    name\n    position\n  }\n}\n"
  }
};
})();

(node as any).hash = "30705cc54a921b8e05588600260e27cf";

export default node;
