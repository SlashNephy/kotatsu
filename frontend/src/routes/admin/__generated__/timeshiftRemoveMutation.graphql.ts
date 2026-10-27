/**
 * @generated SignedSource<<09415d79494eda1f8a81350e0e4f0bd2>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type timeshiftRemoveMutation$variables = {
  serviceId: string;
};
export type timeshiftRemoveMutation$data = {
  readonly removeTimeshiftChannel: string;
};
export type timeshiftRemoveMutation = {
  response: timeshiftRemoveMutation$data;
  variables: timeshiftRemoveMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "serviceId"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "serviceId",
        "variableName": "serviceId"
      }
    ],
    "kind": "ScalarField",
    "name": "removeTimeshiftChannel",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "timeshiftRemoveMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "timeshiftRemoveMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "df589276b3a972115fed64217cb14103",
    "id": null,
    "metadata": {},
    "name": "timeshiftRemoveMutation",
    "operationKind": "mutation",
    "text": "mutation timeshiftRemoveMutation(\n  $serviceId: ID!\n) {\n  removeTimeshiftChannel(serviceId: $serviceId)\n}\n"
  }
};
})();

(node as any).hash = "484224e206e1d9ec8d66ae13a53d389e";

export default node;
