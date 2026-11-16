/**
 * @generated SignedSource<<a4b54c2422bde9d77dd280ff0ec8e83a>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type rulesSetManyGroupMutation$variables = {
  groupId?: string | null | undefined;
  ids: ReadonlyArray<string>;
};
export type rulesSetManyGroupMutation$data = {
  readonly setRulesGroup: {
    readonly processed: number;
    readonly skipped: number;
  };
};
export type rulesSetManyGroupMutation = {
  response: rulesSetManyGroupMutation$data;
  variables: rulesSetManyGroupMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "groupId"
},
v1 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "ids"
},
v2 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "groupId",
        "variableName": "groupId"
      },
      {
        "kind": "Variable",
        "name": "ids",
        "variableName": "ids"
      }
    ],
    "concreteType": "BulkOperationPayload",
    "kind": "LinkedField",
    "name": "setRulesGroup",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "processed",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "skipped",
        "storageKey": null
      }
    ],
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": [
      (v0/*:: as any*/),
      (v1/*:: as any*/)
    ],
    "kind": "Fragment",
    "metadata": null,
    "name": "rulesSetManyGroupMutation",
    "selections": (v2/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [
      (v1/*:: as any*/),
      (v0/*:: as any*/)
    ],
    "kind": "Operation",
    "name": "rulesSetManyGroupMutation",
    "selections": (v2/*:: as any*/)
  },
  "params": {
    "cacheID": "a02e1bf2dbd48c194ea3e8ec5728882d",
    "id": null,
    "metadata": {},
    "name": "rulesSetManyGroupMutation",
    "operationKind": "mutation",
    "text": "mutation rulesSetManyGroupMutation(\n  $ids: [ID!]!\n  $groupId: ID\n) {\n  setRulesGroup(ids: $ids, groupId: $groupId) {\n    processed\n    skipped\n  }\n}\n"
  }
};
})();

(node as any).hash = "76e784d24e51cef9ee107ce399a1df10";

export default node;
