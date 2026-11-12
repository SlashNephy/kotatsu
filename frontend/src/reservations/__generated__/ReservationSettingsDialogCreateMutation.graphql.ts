/**
 * @generated SignedSource<<646ca19bd61766dad54869065ca7590d>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
import { FragmentRefs } from "relay-runtime";
export type AssignmentState = "CONFLICT" | "OK" | "PARTIAL" | "PENDING" | "%future added value";
export type ReservationKind = "PROGRAM" | "RULE" | "TIME" | "%future added value";
export type RecordingSettingsOverridesInput = {
  detectCm?: boolean | null | undefined;
  endMarginSeconds?: number | null | undefined;
  fileNameTemplate?: string | null | undefined;
  joinSegments?: boolean | null | undefined;
  poolId?: string | null | undefined;
  priority?: number | null | undefined;
  startMarginSeconds?: number | null | undefined;
  transcodeFrameRate?: string | null | undefined;
  transcodePreset?: string | null | undefined;
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
      readonly " $fragmentSpreads": FragmentRefs<"ReservationPeers_assignment">;
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
v3 = [
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
v4 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v5 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "enabled",
  "storageKey": null
},
v6 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "kind",
  "storageKey": null
},
v7 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "state",
  "storageKey": null
},
v8 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "plannedStartAt",
  "storageKey": null
},
v9 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "plannedEndAt",
  "storageKey": null
},
v10 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v11 = {
  "alias": null,
  "args": null,
  "concreteType": "Rule",
  "kind": "LinkedField",
  "name": "rules",
  "plural": true,
  "selections": [
    (v4/*:: as any*/),
    (v10/*:: as any*/)
  ],
  "storageKey": null
};
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
    "selections": [
      {
        "alias": null,
        "args": (v3/*:: as any*/),
        "concreteType": "Reservation",
        "kind": "LinkedField",
        "name": "createProgramReservation",
        "plural": false,
        "selections": [
          (v4/*:: as any*/),
          (v5/*:: as any*/),
          (v6/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "concreteType": "ReservationAssignment",
            "kind": "LinkedField",
            "name": "assignment",
            "plural": false,
            "selections": [
              (v7/*:: as any*/),
              (v8/*:: as any*/),
              (v9/*:: as any*/),
              {
                "args": null,
                "kind": "FragmentSpread",
                "name": "ReservationPeers_assignment"
              }
            ],
            "storageKey": null
          },
          (v11/*:: as any*/)
        ],
        "storageKey": null
      }
    ],
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
    "selections": [
      {
        "alias": null,
        "args": (v3/*:: as any*/),
        "concreteType": "Reservation",
        "kind": "LinkedField",
        "name": "createProgramReservation",
        "plural": false,
        "selections": [
          (v4/*:: as any*/),
          (v5/*:: as any*/),
          (v6/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "concreteType": "ReservationAssignment",
            "kind": "LinkedField",
            "name": "assignment",
            "plural": false,
            "selections": [
              (v7/*:: as any*/),
              (v8/*:: as any*/),
              (v9/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "Reservation",
                "kind": "LinkedField",
                "name": "peers",
                "plural": true,
                "selections": [
                  (v4/*:: as any*/),
                  (v10/*:: as any*/),
                  {
                    "alias": null,
                    "args": null,
                    "concreteType": "Service",
                    "kind": "LinkedField",
                    "name": "service",
                    "plural": false,
                    "selections": [
                      (v10/*:: as any*/),
                      (v4/*:: as any*/)
                    ],
                    "storageKey": null
                  }
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          },
          (v11/*:: as any*/)
        ],
        "storageKey": null
      }
    ]
  },
  "params": {
    "cacheID": "639ffaf6419ba8b67a90e7e8d13e68e2",
    "id": null,
    "metadata": {},
    "name": "ReservationSettingsDialogCreateMutation",
    "operationKind": "mutation",
    "text": "mutation ReservationSettingsDialogCreateMutation(\n  $program: ID!\n  $settings: RecordingSettingsOverridesInput!\n  $followEventRelay: Boolean!\n) {\n  createProgramReservation(input: {program: $program, settings: $settings, followEventRelay: $followEventRelay}) {\n    id\n    enabled\n    kind\n    assignment {\n      state\n      plannedStartAt\n      plannedEndAt\n      ...ReservationPeers_assignment\n    }\n    rules {\n      id\n      name\n    }\n  }\n}\n\nfragment ReservationPeers_assignment on ReservationAssignment {\n  peers {\n    id\n    name\n    service {\n      name\n      id\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "55b92788ae8e428197787b2a2becad66";

export default node;
