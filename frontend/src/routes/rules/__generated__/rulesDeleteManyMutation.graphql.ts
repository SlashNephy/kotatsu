/**
 * @generated SignedSource<<a210c9828b52cf362071084b054571eb>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type rulesDeleteManyMutation$variables = {
  ids: ReadonlyArray<string>;
};
export type rulesDeleteManyMutation$data = {
  readonly deleteRules: {
    readonly processed: number;
    readonly skipped: number;
  };
};
export type rulesDeleteManyMutation = {
  response: rulesDeleteManyMutation$data;
  variables: rulesDeleteManyMutation$variables;
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
    "concreteType": "BulkOperationPayload",
    "kind": "LinkedField",
    "name": "deleteRules",
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
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "rulesDeleteManyMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "rulesDeleteManyMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "46358eb6c7b27d3a8a7ecde46c880758",
    "id": null,
    "metadata": {},
    "name": "rulesDeleteManyMutation",
    "operationKind": "mutation",
    "text": "mutation rulesDeleteManyMutation(\n  $ids: [ID!]!\n) {\n  deleteRules(ids: $ids) {\n    processed\n    skipped\n  }\n}\n"
  }
};
})();

(node as any).hash = "ad12dae3652470eb48a09d283235f06c";

export default node;
