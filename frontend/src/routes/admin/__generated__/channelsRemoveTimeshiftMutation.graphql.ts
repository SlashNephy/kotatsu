/**
 * @generated SignedSource<<2d47395a190a4483ad0ce54c79be4333>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type channelsRemoveTimeshiftMutation$variables = {
  serviceId: string;
};
export type channelsRemoveTimeshiftMutation$data = {
  readonly removeTimeshiftChannel: string;
};
export type channelsRemoveTimeshiftMutation = {
  response: channelsRemoveTimeshiftMutation$data;
  variables: channelsRemoveTimeshiftMutation$variables;
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
    "name": "channelsRemoveTimeshiftMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "channelsRemoveTimeshiftMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "b41c58a32ad9a5000399a646fec82be2",
    "id": null,
    "metadata": {},
    "name": "channelsRemoveTimeshiftMutation",
    "operationKind": "mutation",
    "text": "mutation channelsRemoveTimeshiftMutation(\n  $serviceId: ID!\n) {\n  removeTimeshiftChannel(serviceId: $serviceId)\n}\n"
  }
};
})();

(node as any).hash = "96abeebdd9d3622485a31c6786bb417e";

export default node;
