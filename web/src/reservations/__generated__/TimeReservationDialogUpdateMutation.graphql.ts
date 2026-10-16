/**
 * @generated SignedSource<<18bf5337cf92a95fd667f9f6c9308d31>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type ReservationUpdateInput = {
  enabled?: boolean | null | undefined;
  endAt?: string | null | undefined;
  followEventRelay?: boolean | null | undefined;
  name?: string | null | undefined;
  settings?: RecordingSettingsOverridesInput | null | undefined;
  startAt?: string | null | undefined;
};
export type RecordingSettingsOverridesInput = {
  endMarginSeconds?: number | null | undefined;
  fileNameTemplate?: string | null | undefined;
  joinSegments?: boolean | null | undefined;
  poolId?: string | null | undefined;
  priority?: number | null | undefined;
  startMarginSeconds?: number | null | undefined;
};
export type TimeReservationDialogUpdateMutation$variables = {
  id: string;
  input: ReservationUpdateInput;
};
export type TimeReservationDialogUpdateMutation$data = {
  readonly updateReservation: {
    readonly endAt: string;
    readonly id: string;
    readonly name: string;
    readonly startAt: string;
  };
};
export type TimeReservationDialogUpdateMutation = {
  response: TimeReservationDialogUpdateMutation$data;
  variables: TimeReservationDialogUpdateMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "id"
  },
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "input"
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
      },
      {
        "kind": "Variable",
        "name": "input",
        "variableName": "input"
      }
    ],
    "concreteType": "Reservation",
    "kind": "LinkedField",
    "name": "updateReservation",
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
        "name": "startAt",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "endAt",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "name",
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
    "name": "TimeReservationDialogUpdateMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "TimeReservationDialogUpdateMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "4b232e2c577cc6c7c179e7efd3892934",
    "id": null,
    "metadata": {},
    "name": "TimeReservationDialogUpdateMutation",
    "operationKind": "mutation",
    "text": "mutation TimeReservationDialogUpdateMutation(\n  $id: ID!\n  $input: ReservationUpdateInput!\n) {\n  updateReservation(id: $id, input: $input) {\n    id\n    startAt\n    endAt\n    name\n  }\n}\n"
  }
};
})();

(node as any).hash = "90737c1b27fbbc830a26b55ab6f3cab3";

export default node;
