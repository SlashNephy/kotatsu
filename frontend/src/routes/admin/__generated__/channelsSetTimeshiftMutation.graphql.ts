/**
 * @generated SignedSource<<10b6a51a3476b70bf4e03cdb2f3b8a13>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type channelsSetTimeshiftMutation$variables = {
  retentionSeconds: number;
  serviceId: string;
};
export type channelsSetTimeshiftMutation$data = {
  readonly setTimeshiftChannel: {
    readonly retentionSeconds: number;
  };
};
export type channelsSetTimeshiftMutation = {
  response: channelsSetTimeshiftMutation$data;
  variables: channelsSetTimeshiftMutation$variables;
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
    "name": "channelsSetTimeshiftMutation",
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
    "name": "channelsSetTimeshiftMutation",
    "selections": (v2/*:: as any*/)
  },
  "params": {
    "cacheID": "4eaf79d0648c8ed89a7bbd8d5e59c10e",
    "id": null,
    "metadata": {},
    "name": "channelsSetTimeshiftMutation",
    "operationKind": "mutation",
    "text": "mutation channelsSetTimeshiftMutation(\n  $serviceId: ID!\n  $retentionSeconds: Int!\n) {\n  setTimeshiftChannel(serviceId: $serviceId, retentionSeconds: $retentionSeconds) {\n    retentionSeconds\n  }\n}\n"
  }
};
})();

(node as any).hash = "bf9fc3fc5ca2448e514a2755ffcaf43d";

export default node;
