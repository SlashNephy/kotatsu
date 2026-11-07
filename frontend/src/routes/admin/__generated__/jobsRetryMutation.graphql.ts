/**
 * @generated SignedSource<<fba1b873fd65b9ad01c0bf6910786379>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type JobState = "AVAILABLE" | "CANCELLED" | "COMPLETED" | "DISCARDED" | "PENDING" | "RETRYABLE" | "RUNNING" | "SCHEDULED" | "%future added value";
export type jobsRetryMutation$variables = {
  id: string;
};
export type jobsRetryMutation$data = {
  readonly retryJob: {
    readonly id: string;
    readonly state: JobState;
  };
};
export type jobsRetryMutation = {
  response: jobsRetryMutation$data;
  variables: jobsRetryMutation$variables;
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
    "concreteType": "Job",
    "kind": "LinkedField",
    "name": "retryJob",
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
        "name": "state",
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
    "name": "jobsRetryMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "jobsRetryMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "28f223d68e415c86acf12c03c947bf79",
    "id": null,
    "metadata": {},
    "name": "jobsRetryMutation",
    "operationKind": "mutation",
    "text": "mutation jobsRetryMutation(\n  $id: ID!\n) {\n  retryJob(id: $id) {\n    id\n    state\n  }\n}\n"
  }
};
})();

(node as any).hash = "aad5ebd2fe19323a183eeddb1a290970";

export default node;
