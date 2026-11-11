/**
 * @generated SignedSource<<d3ee8c6dcffcc012670f4162a273e730>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
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
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v2 = [
  {
    "alias": null,
    "args": [
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
    "concreteType": "Reservation",
    "kind": "LinkedField",
    "name": "createProgramReservation",
    "plural": false,
    "selections": [
      (v1/*:: as any*/),
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
          (v1/*:: as any*/),
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
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "ProgramDetailReserveMutation",
    "selections": (v2/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "ProgramDetailReserveMutation",
    "selections": (v2/*:: as any*/)
  },
  "params": {
    "cacheID": "c9e36555e4a0d75c4386f392c78b3871",
    "id": null,
    "metadata": {},
    "name": "ProgramDetailReserveMutation",
    "operationKind": "mutation",
    "text": "mutation ProgramDetailReserveMutation(\n  $program: ID!\n) {\n  createProgramReservation(input: {program: $program}) {\n    id\n    enabled\n    kind\n    assignment {\n      state\n      plannedStartAt\n      plannedEndAt\n    }\n    rules {\n      id\n      name\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "57014fd6735a4307f9c03f274a095c5a";

export default node;
