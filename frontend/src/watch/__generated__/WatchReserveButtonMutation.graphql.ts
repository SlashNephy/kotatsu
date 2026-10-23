/**
 * @generated SignedSource<<2a0ef5fec7b7725a3ac24c2547cf1f32>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type WatchReserveButtonMutation$variables = {
  program: string;
  reserved: boolean;
};
export type WatchReserveButtonMutation$data = {
  readonly setWatchReservation: {
    readonly id: string;
    readonly watchReserved: boolean;
  };
};
export type WatchReserveButtonMutation = {
  response: WatchReserveButtonMutation$data;
  variables: WatchReserveButtonMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "program"
  },
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "reserved"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "program",
        "variableName": "program"
      },
      {
        "kind": "Variable",
        "name": "reserved",
        "variableName": "reserved"
      }
    ],
    "concreteType": "Program",
    "kind": "LinkedField",
    "name": "setWatchReservation",
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
        "name": "watchReserved",
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
    "name": "WatchReserveButtonMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "WatchReserveButtonMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "12e962add4a15a11a739fa3911a0d9c3",
    "id": null,
    "metadata": {},
    "name": "WatchReserveButtonMutation",
    "operationKind": "mutation",
    "text": "mutation WatchReserveButtonMutation(\n  $program: ID!\n  $reserved: Boolean!\n) {\n  setWatchReservation(program: $program, reserved: $reserved) {\n    id\n    watchReserved\n  }\n}\n"
  }
};
})();

(node as any).hash = "a339cc897898fc1887fcdc23c2b1b65c";

export default node;
