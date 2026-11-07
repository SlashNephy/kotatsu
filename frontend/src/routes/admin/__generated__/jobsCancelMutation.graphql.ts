/**
 * @generated SignedSource<<f1567021ddf93dc68b3411a9dfafbb2f>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type JobState = "AVAILABLE" | "CANCELLED" | "COMPLETED" | "DISCARDED" | "PENDING" | "RETRYABLE" | "RUNNING" | "SCHEDULED" | "%future added value";
export type jobsCancelMutation$variables = {
  id: string;
};
export type jobsCancelMutation$data = {
  readonly cancelJob: {
    readonly id: string;
    readonly state: JobState;
  };
};
export type jobsCancelMutation = {
  response: jobsCancelMutation$data;
  variables: jobsCancelMutation$variables;
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
    "name": "cancelJob",
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
    "name": "jobsCancelMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "jobsCancelMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "96db569c2733e862367dfaaba54d3d4a",
    "id": null,
    "metadata": {},
    "name": "jobsCancelMutation",
    "operationKind": "mutation",
    "text": "mutation jobsCancelMutation(\n  $id: ID!\n) {\n  cancelJob(id: $id) {\n    id\n    state\n  }\n}\n"
  }
};
})();

(node as any).hash = "3e2d8af5d1dc65bf51a5c9977b8fa17a";

export default node;
