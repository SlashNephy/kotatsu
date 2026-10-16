/**
 * @generated SignedSource<<2948b90867f50f68a39d98507fd8d84f>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type TimeReservationInput = {
  endAt: string;
  name?: string;
  service: string;
  settings?: RecordingSettingsOverridesInput | null | undefined;
  startAt: string;
};
export type RecordingSettingsOverridesInput = {
  endMarginSeconds?: number | null | undefined;
  fileNameTemplate?: string | null | undefined;
  joinSegments?: boolean | null | undefined;
  poolId?: string | null | undefined;
  priority?: number | null | undefined;
  startMarginSeconds?: number | null | undefined;
};
export type TimeReservationDialogCreateMutation$variables = {
  input: TimeReservationInput;
};
export type TimeReservationDialogCreateMutation$data = {
  readonly createTimeReservation: {
    readonly id: string;
  };
};
export type TimeReservationDialogCreateMutation = {
  response: TimeReservationDialogCreateMutation$data;
  variables: TimeReservationDialogCreateMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
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
        "name": "input",
        "variableName": "input"
      }
    ],
    "concreteType": "Reservation",
    "kind": "LinkedField",
    "name": "createTimeReservation",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "id",
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
    "name": "TimeReservationDialogCreateMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "TimeReservationDialogCreateMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "5e9a4221039db279e0a2b69f6e251fd3",
    "id": null,
    "metadata": {},
    "name": "TimeReservationDialogCreateMutation",
    "operationKind": "mutation",
    "text": "mutation TimeReservationDialogCreateMutation(\n  $input: TimeReservationInput!\n) {\n  createTimeReservation(input: $input) {\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "4c34534e363cabd80325bee4caa9cd70";

export default node;
