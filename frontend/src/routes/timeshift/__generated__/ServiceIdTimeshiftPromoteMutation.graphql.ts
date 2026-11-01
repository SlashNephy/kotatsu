/**
 * @generated SignedSource<<a1aa93ae2d0b3331f9296da69e3d05e7>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type ServiceIdTimeshiftPromoteMutation$variables = {
  serviceId: string;
  startAt: string;
};
export type ServiceIdTimeshiftPromoteMutation$data = {
  readonly promoteTimeshiftProgram: {
    readonly recording: {
      readonly id: string;
    } | null | undefined;
    readonly reservation: {
      readonly id: string;
    } | null | undefined;
  };
};
export type ServiceIdTimeshiftPromoteMutation = {
  response: ServiceIdTimeshiftPromoteMutation$data;
  variables: ServiceIdTimeshiftPromoteMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "serviceId"
  },
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "startAt"
  }
],
v1 = [
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "id",
    "storageKey": null
  }
],
v2 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "serviceId",
        "variableName": "serviceId"
      },
      {
        "kind": "Variable",
        "name": "startAt",
        "variableName": "startAt"
      }
    ],
    "concreteType": "TimeshiftPromotion",
    "kind": "LinkedField",
    "name": "promoteTimeshiftProgram",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "Recording",
        "kind": "LinkedField",
        "name": "recording",
        "plural": false,
        "selections": (v1/*:: as any*/),
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "Reservation",
        "kind": "LinkedField",
        "name": "reservation",
        "plural": false,
        "selections": (v1/*:: as any*/),
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
    "name": "ServiceIdTimeshiftPromoteMutation",
    "selections": (v2/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "ServiceIdTimeshiftPromoteMutation",
    "selections": (v2/*:: as any*/)
  },
  "params": {
    "cacheID": "49bfd526172dfd055ab025ca7c613241",
    "id": null,
    "metadata": {},
    "name": "ServiceIdTimeshiftPromoteMutation",
    "operationKind": "mutation",
    "text": "mutation ServiceIdTimeshiftPromoteMutation(\n  $serviceId: ID!\n  $startAt: Time!\n) {\n  promoteTimeshiftProgram(serviceId: $serviceId, startAt: $startAt) {\n    recording {\n      id\n    }\n    reservation {\n      id\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "f7a3fd77e522c96be86757e4a06480a9";

export default node;
