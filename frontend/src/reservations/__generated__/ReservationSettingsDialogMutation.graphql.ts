/**
 * @generated SignedSource<<636d7911a61332a70c0509b56d882849>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type AssignmentState = "CONFLICT" | "OK" | "PARTIAL" | "PENDING" | "%future added value";
export type RecordingSettingsOverridesInput = {
  detectCm?: boolean | null | undefined;
  endMarginSeconds?: number | null | undefined;
  fileNameTemplate?: string | null | undefined;
  joinSegments?: boolean | null | undefined;
  poolId?: string | null | undefined;
  priority?: number | null | undefined;
  startMarginSeconds?: number | null | undefined;
};
export type ReservationSettingsDialogMutation$variables = {
  followEventRelay?: boolean | null | undefined;
  id: string;
  settings: RecordingSettingsOverridesInput;
};
export type ReservationSettingsDialogMutation$data = {
  readonly updateReservation: {
    readonly assignment: {
      readonly state: AssignmentState;
    };
    readonly followEventRelay: boolean;
    readonly id: string;
    readonly settings: {
      readonly detectCm: boolean | null | undefined;
      readonly endMarginSeconds: number | null | undefined;
      readonly fileNameTemplate: string | null | undefined;
      readonly joinSegments: boolean | null | undefined;
      readonly poolId: string | null | undefined;
      readonly priority: number | null | undefined;
      readonly startMarginSeconds: number | null | undefined;
    };
  };
};
export type ReservationSettingsDialogMutation = {
  response: ReservationSettingsDialogMutation$data;
  variables: ReservationSettingsDialogMutation$variables;
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
  "name": "id"
},
v2 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "settings"
},
v3 = [
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
            "name": "followEventRelay",
            "variableName": "followEventRelay"
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
        "name": "followEventRelay",
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
            "name": "poolId",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "fileNameTemplate",
            "storageKey": null
          },
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
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "priority",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "joinSegments",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "detectCm",
            "storageKey": null
          }
        ],
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
    "name": "ReservationSettingsDialogMutation",
    "selections": (v3/*:: as any*/),
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
    "name": "ReservationSettingsDialogMutation",
    "selections": (v3/*:: as any*/)
  },
  "params": {
    "cacheID": "11852ce87590c9223fe174298cdf0cf2",
    "id": null,
    "metadata": {},
    "name": "ReservationSettingsDialogMutation",
    "operationKind": "mutation",
    "text": "mutation ReservationSettingsDialogMutation(\n  $id: ID!\n  $settings: RecordingSettingsOverridesInput!\n  $followEventRelay: Boolean\n) {\n  updateReservation(id: $id, input: {settings: $settings, followEventRelay: $followEventRelay}) {\n    id\n    followEventRelay\n    settings {\n      poolId\n      fileNameTemplate\n      startMarginSeconds\n      endMarginSeconds\n      priority\n      joinSegments\n      detectCm\n    }\n    assignment {\n      state\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "e0dabee608d95c85fdcf29ad9f41a52c";

export default node;
