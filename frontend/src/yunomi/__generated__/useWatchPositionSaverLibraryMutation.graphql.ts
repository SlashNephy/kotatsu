/**
 * @generated SignedSource<<6b515ee7fdfdc6648f66b5a5f8ed6506>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type useWatchPositionSaverLibraryMutation$variables = {
  id: string;
  positionMs: number;
};
export type useWatchPositionSaverLibraryMutation$data = {
  readonly saveLibraryWatchPosition: {
    readonly id: string;
    readonly watchPositionMs: number | null | undefined;
  };
};
export type useWatchPositionSaverLibraryMutation = {
  response: useWatchPositionSaverLibraryMutation$data;
  variables: useWatchPositionSaverLibraryMutation$variables;
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
    "concreteType": "LibraryFile",
    "kind": "LinkedField",
    "name": "saveLibraryWatchPosition",
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
    "name": "useWatchPositionSaverLibraryMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "useWatchPositionSaverLibraryMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "f70bc0aef532016d960b754c5669be09",
    "id": null,
    "metadata": {},
    "name": "useWatchPositionSaverLibraryMutation",
    "operationKind": "mutation",
    "text": "mutation useWatchPositionSaverLibraryMutation(\n  $id: ID!\n  $positionMs: Int64!\n) {\n  saveLibraryWatchPosition(id: $id, positionMs: $positionMs) {\n    id\n    watchPositionMs\n  }\n}\n"
  }
};
})();

(node as any).hash = "2c3b225e1d86d8515787ec178106bdae";

export default node;
