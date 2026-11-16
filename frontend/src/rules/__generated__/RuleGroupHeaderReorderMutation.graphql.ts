/**
 * @generated SignedSource<<8d738d811eb09275d10adb579784f24f>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type RuleGroupHeaderReorderMutation$variables = {
  ids: ReadonlyArray<string>;
};
export type RuleGroupHeaderReorderMutation$data = {
  readonly reorderRuleGroups: ReadonlyArray<{
    readonly id: string;
    readonly name: string;
    readonly position: number;
  }>;
};
export type RuleGroupHeaderReorderMutation = {
  response: RuleGroupHeaderReorderMutation$data;
  variables: RuleGroupHeaderReorderMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "ids"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "ids",
        "variableName": "ids"
      }
    ],
    "concreteType": "RuleGroup",
    "kind": "LinkedField",
    "name": "reorderRuleGroups",
    "plural": true,
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
    "name": "RuleGroupHeaderReorderMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "RuleGroupHeaderReorderMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "d835178cc40ca99f31ba5fea8a2eef4b",
    "id": null,
    "metadata": {},
    "name": "RuleGroupHeaderReorderMutation",
    "operationKind": "mutation",
    "text": "mutation RuleGroupHeaderReorderMutation(\n  $ids: [ID!]!\n) {\n  reorderRuleGroups(ids: $ids) {\n    id\n    name\n    position\n  }\n}\n"
  }
};
})();

(node as any).hash = "7fc36538e817be7a885c614afd881e6d";

export default node;
