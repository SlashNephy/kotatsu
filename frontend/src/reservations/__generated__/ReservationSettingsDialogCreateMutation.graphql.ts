/**
 * @generated SignedSource<<621fb4f72f469829f934270c89b254ec>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type AssignmentState = "CONFLICT" | "OK" | "PARTIAL" | "PENDING" | "%future added value";
export type ReservationKind = "PROGRAM" | "RULE" | "TIME" | "%future added value";
export type RecordingSettingsOverridesInput = {
  endMarginSeconds?: number | null | undefined;
  fileNameTemplate?: string | null | undefined;
  joinSegments?: boolean | null | undefined;
  poolId?: string | null | undefined;
  priority?: number | null | undefined;
  startMarginSeconds?: number | null | undefined;
};
export type ReservationSettingsDialogCreateMutation$variables = {
  followEventRelay: boolean;
  program: string;
  settings: RecordingSettingsOverridesInput;
};
export type ReservationSettingsDialogCreateMutation$data = {
  readonly createProgramReservation: {
    readonly assignment: {
      readonly plannedEndAt: string | null | undefined;
      readonly plannedStartAt: string | null | undefined;
      readonly state: AssignmentState;
    };
    readonly enabled: boolean;
    readonly id: string;
    readonly kind: ReservationKind;
    readonly rules: ReadonlyArray<{
      readonly id: string;
      readonly name: string;
    }>;
  };
};
export type ReservationSettingsDialogCreateMutation = {
  response: ReservationSettingsDialogCreateMutation$data;
  variables: ReservationSettingsDialogCreateMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "followEventRelay"
},
v1 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "program"
},
v2 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "settings"
},
v3 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v4 = [
  {
    "alias": null,
    "args": [
      {
        "fields": [
          {
            "kind": "Variable",
            "name": "followEventRelay",
            "variableName": "followEventRelay"
          },
          {
            "kind": "Variable",
            "name": "program",
            "variableName": "program"
          },
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
    "name": "createProgramReservation",
    "plural": false,
    "selections": [
      (v3/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "enabled",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "kind",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "ReservationAssignment",
        "kind": "LinkedField",
        "name": "assignment",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "state",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "plannedStartAt",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "plannedEndAt",
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "Rule",
        "kind": "LinkedField",
        "name": "rules",
        "plural": true,
        "selections": [
          (v3/*:: as any*/),
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
    ],
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": [
      (v0/*:: as any*/),
      (v1/*:: as any*/),
      (v2/*:: as any*/)
    ],
    "kind": "Fragment",
    "metadata": null,
    "name": "ReservationSettingsDialogCreateMutation",
    "selections": (v4/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [
      (v1/*:: as any*/),
      (v2/*:: as any*/),
      (v0/*:: as any*/)
    ],
    "kind": "Operation",
    "name": "ReservationSettingsDialogCreateMutation",
    "selections": (v4/*:: as any*/)
  },
  "params": {
    "cacheID": "25e35bd17f43c463a7b8f4b978568777",
    "id": null,
    "metadata": {},
    "name": "ReservationSettingsDialogCreateMutation",
    "operationKind": "mutation",
    "text": "mutation ReservationSettingsDialogCreateMutation(\n  $program: ID!\n  $settings: RecordingSettingsOverridesInput!\n  $followEventRelay: Boolean!\n) {\n  createProgramReservation(input: {program: $program, settings: $settings, followEventRelay: $followEventRelay}) {\n    id\n    enabled\n    kind\n    assignment {\n      state\n      plannedStartAt\n      plannedEndAt\n    }\n    rules {\n      id\n      name\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "d9548123815cc129a734576711907b0d";

export default node;
