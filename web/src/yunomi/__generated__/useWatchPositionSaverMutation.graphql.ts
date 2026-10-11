/**
 * @generated SignedSource<<0fbd50a0a221da403ef9c0be1381c457>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type useWatchPositionSaverMutation$variables = {
  id: string;
  positionMs: any;
};
export type useWatchPositionSaverMutation$data = {
  readonly saveWatchPosition: {
    readonly id: string;
    readonly watchPositionMs: any | null | undefined;
  };
};
export type useWatchPositionSaverMutation = {
  response: useWatchPositionSaverMutation$data;
  variables: useWatchPositionSaverMutation$variables;
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
    "name": "positionMs"
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
        "name": "positionMs",
        "variableName": "positionMs"
      }
    ],
    "concreteType": "Recording",
    "kind": "LinkedField",
    "name": "saveWatchPosition",
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
        "name": "watchPositionMs",
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
    "name": "useWatchPositionSaverMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "useWatchPositionSaverMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "da11ac502258297f189efc21b6c1d0ad",
    "id": null,
    "metadata": {},
    "name": "useWatchPositionSaverMutation",
    "operationKind": "mutation",
    "text": "mutation useWatchPositionSaverMutation(\n  $id: ID!\n  $positionMs: Int64!\n) {\n  saveWatchPosition(id: $id, positionMs: $positionMs) {\n    id\n    watchPositionMs\n  }\n}\n"
  }
};
})();

(node as any).hash = "ef87e78ad1df081dac84f72aac3be0eb";

export default node;
