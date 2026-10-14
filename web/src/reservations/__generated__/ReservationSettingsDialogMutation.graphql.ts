/**
 * @generated SignedSource<<4bed718aaec56e5c47e157eb22113e35>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type RecordingSettingsOverridesInput = {
  endMarginSeconds?: number | null | undefined;
  fileNameTemplate?: string | null | undefined;
  joinSegments?: boolean | null | undefined;
  poolId?: string | null | undefined;
  priority?: number | null | undefined;
  startMarginSeconds?: number | null | undefined;
};
export type ReservationSettingsDialogMutation$variables = {
  id: string;
  settings: RecordingSettingsOverridesInput;
};
export type ReservationSettingsDialogMutation$data = {
  readonly updateReservation: {
    readonly id: string;
    readonly settings: {
      readonly endMarginSeconds: number | null | undefined;
      readonly startMarginSeconds: number | null | undefined;
    };
  };
};
export type ReservationSettingsDialogMutation = {
  response: ReservationSettingsDialogMutation$data;
  variables: ReservationSettingsDialogMutation$variables;
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
    "name": "settings"
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
        "fields": [
          {
            "kind": "Variable",
            "name": "settings",
            "variableName": "settings"
          }
        ],
        "kind": "ObjectValue",
        "name": "input"
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
        "concreteType": "RecordingSettingsOverrides",
        "kind": "LinkedField",
        "name": "settings",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "startMarginSeconds",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "endMarginSeconds",
            "storageKey": null
          }
        ],
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
    "name": "ReservationSettingsDialogMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "ReservationSettingsDialogMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "939826699939e04d6e0a14369e995027",
    "id": null,
    "metadata": {},
    "name": "ReservationSettingsDialogMutation",
    "operationKind": "mutation",
    "text": "mutation ReservationSettingsDialogMutation(\n  $id: ID!\n  $settings: RecordingSettingsOverridesInput!\n) {\n  updateReservation(id: $id, input: {settings: $settings}) {\n    id\n    settings {\n      startMarginSeconds\n      endMarginSeconds\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "0d22a292c53eb4e8f99942ff8a885057";

export default node;
