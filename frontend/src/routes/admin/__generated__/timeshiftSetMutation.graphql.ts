/**
 * @generated SignedSource<<5f9d6cd2b7e3f2373ca73b17847eb637>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type timeshiftSetMutation$variables = {
  retentionSeconds: number;
  serviceId: string;
};
export type timeshiftSetMutation$data = {
  readonly setTimeshiftChannel: {
    readonly retentionSeconds: number;
  };
};
export type timeshiftSetMutation = {
  response: timeshiftSetMutation$data;
  variables: timeshiftSetMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "retentionSeconds"
},
v1 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "serviceId"
},
v2 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "retentionSeconds",
        "variableName": "retentionSeconds"
      },
      {
        "kind": "Variable",
        "name": "serviceId",
        "variableName": "serviceId"
      }
    ],
    "concreteType": "TimeshiftChannel",
    "kind": "LinkedField",
    "name": "setTimeshiftChannel",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "retentionSeconds",
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
    "name": "timeshiftSetMutation",
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
    "name": "timeshiftSetMutation",
    "selections": (v2/*:: as any*/)
  },
  "params": {
    "cacheID": "a88c2b1ffe1201aee7849be3ad8f2cde",
    "id": null,
    "metadata": {},
    "name": "timeshiftSetMutation",
    "operationKind": "mutation",
    "text": "mutation timeshiftSetMutation(\n  $serviceId: ID!\n  $retentionSeconds: Int!\n) {\n  setTimeshiftChannel(serviceId: $serviceId, retentionSeconds: $retentionSeconds) {\n    retentionSeconds\n  }\n}\n"
  }
};
})();

(node as any).hash = "1f0a7a8b6361e166625af6a55ba10d22";

export default node;
