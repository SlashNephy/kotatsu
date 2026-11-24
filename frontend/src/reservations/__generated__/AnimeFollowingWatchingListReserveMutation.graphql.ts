/**
 * @generated SignedSource<<b9be5c6950cd3615f4e25d7a7a21d908>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type AnimeFollowingWatchingListReserveMutation$variables = {
  program: string;
};
export type AnimeFollowingWatchingListReserveMutation$data = {
  readonly createProgramReservation: {
    readonly id: string;
  };
};
export type AnimeFollowingWatchingListReserveMutation = {
  response: AnimeFollowingWatchingListReserveMutation$data;
  variables: AnimeFollowingWatchingListReserveMutation$variables;
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
    "name": "AnimeFollowingWatchingListReserveMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "AnimeFollowingWatchingListReserveMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "bdd2256c623ec114ae5d38e0eb10c396",
    "id": null,
    "metadata": {},
    "name": "AnimeFollowingWatchingListReserveMutation",
    "operationKind": "mutation",
    "text": "mutation AnimeFollowingWatchingListReserveMutation(\n  $program: ID!\n) {\n  createProgramReservation(input: {program: $program}) {\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "bb55b99ecd28b5fe4353c3c62b0e8ea1";

export default node;
