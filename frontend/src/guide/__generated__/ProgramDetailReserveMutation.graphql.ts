/**
 * @generated SignedSource<<d7923ba00a94f8e810fb81a356d5e25f>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
import { FragmentRefs } from "relay-runtime";
export type AssignmentState = "CONFLICT" | "OK" | "PARTIAL" | "PENDING" | "%future added value";
export type ReservationKind = "PROGRAM" | "RULE" | "TIME" | "%future added value";
export type ProgramDetailReserveMutation$variables = {
  program: string;
};
export type ProgramDetailReserveMutation$data = {
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
export type ProgramDetailReserveMutation = {
  response: ProgramDetailReserveMutation$data;
  variables: ProgramDetailReserveMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "program"
  }
],
v1 = [
  {
    "fields": [
      {
        "kind": "Variable",
        "name": "program",
        "variableName": "program"
      }
    ],
    "kind": "ObjectValue",
    "name": "input"
  }
],
v2 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v3 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "enabled",
  "storageKey": null
},
v4 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "kind",
  "storageKey": null
},
v5 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "state",
  "storageKey": null
},
v6 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "plannedStartAt",
  "storageKey": null
},
v7 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "plannedEndAt",
  "storageKey": null
},
v8 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v9 = {
  "alias": null,
  "args": null,
  "concreteType": "Rule",
  "kind": "LinkedField",
  "name": "rules",
  "plural": true,
  "selections": [
    (v2/*:: as any*/),
    (v8/*:: as any*/)
  ],
  "storageKey": null
};
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "ProgramDetailReserveMutation",
    "selections": [
      {
        "alias": null,
        "args": (v1/*:: as any*/),
        "concreteType": "Reservation",
        "kind": "LinkedField",
        "name": "createProgramReservation",
        "plural": false,
        "selections": [
          (v2/*:: as any*/),
          (v3/*:: as any*/),
          (v4/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "concreteType": "ReservationAssignment",
            "kind": "LinkedField",
            "name": "assignment",
            "plural": false,
            "selections": [
              (v5/*:: as any*/),
              (v6/*:: as any*/),
              (v7/*:: as any*/),
              {
                "args": null,
                "kind": "FragmentSpread",
                "name": "ReservationPeers_assignment"
              }
            ],
            "storageKey": null
          },
          (v9/*:: as any*/)
        ],
        "storageKey": null
      }
    ],
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "ProgramDetailReserveMutation",
    "selections": [
      {
        "alias": null,
        "args": (v1/*:: as any*/),
        "concreteType": "Reservation",
        "kind": "LinkedField",
        "name": "createProgramReservation",
        "plural": false,
        "selections": [
          (v2/*:: as any*/),
          (v3/*:: as any*/),
          (v4/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "concreteType": "ReservationAssignment",
            "kind": "LinkedField",
            "name": "assignment",
            "plural": false,
            "selections": [
              (v5/*:: as any*/),
              (v6/*:: as any*/),
              (v7/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "Reservation",
                "kind": "LinkedField",
                "name": "peers",
                "plural": true,
                "selections": [
                  (v2/*:: as any*/),
                  (v8/*:: as any*/),
                  {
                    "alias": null,
                    "args": null,
                    "concreteType": "Service",
                    "kind": "LinkedField",
                    "name": "service",
                    "plural": false,
                    "selections": [
                      (v8/*:: as any*/),
                      (v2/*:: as any*/)
                    ],
                    "storageKey": null
                  }
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          },
          (v9/*:: as any*/)
        ],
        "storageKey": null
      }
    ]
  },
  "params": {
    "cacheID": "557dbc8a4f7e923da72eac9cac80d4a6",
    "id": null,
    "metadata": {},
    "name": "ProgramDetailReserveMutation",
    "operationKind": "mutation",
    "text": "mutation ProgramDetailReserveMutation(\n  $program: ID!\n) {\n  createProgramReservation(input: {program: $program}) {\n    id\n    enabled\n    kind\n    assignment {\n      state\n      plannedStartAt\n      plannedEndAt\n      ...ReservationPeers_assignment\n    }\n    rules {\n      id\n      name\n    }\n  }\n}\n\nfragment ReservationPeers_assignment on ReservationAssignment {\n  peers {\n    id\n    name\n    service {\n      name\n      id\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "1d750acb8bc2550e4c66c11b6f552e5e";

export default node;
