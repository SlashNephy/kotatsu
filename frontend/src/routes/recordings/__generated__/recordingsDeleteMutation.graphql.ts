/**
 * @generated SignedSource<<62d2e4f314ff5962b4b25524670cc452>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type recordingsDeleteMutation$variables = {
  ids: ReadonlyArray<string>;
};
export type recordingsDeleteMutation$data = {
  readonly deleteRecordings: {
    readonly processed: number;
    readonly skipped: number;
  };
};
export type recordingsDeleteMutation = {
  response: recordingsDeleteMutation$data;
  variables: recordingsDeleteMutation$variables;
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
    "name": "deleteRecordings",
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
    "name": "recordingsDeleteMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "recordingsDeleteMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "a3f4d41c4012bc7d5c1743ca19db37e9",
    "id": null,
    "metadata": {},
    "name": "recordingsDeleteMutation",
    "operationKind": "mutation",
    "text": "mutation recordingsDeleteMutation(\n  $ids: [ID!]!\n) {\n  deleteRecordings(ids: $ids) {\n    processed\n    skipped\n  }\n}\n"
  }
};
})();

(node as any).hash = "4060b1b55853ff047e8c658bc16bf86d";

export default node;
