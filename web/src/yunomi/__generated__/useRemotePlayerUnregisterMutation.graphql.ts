/**
 * @generated SignedSource<<1eece9bffb4b20821a6cf6a9305cea35>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type useRemotePlayerUnregisterMutation$variables = {
  id: string;
};
export type useRemotePlayerUnregisterMutation$data = {
  readonly unregisterPlayer: string;
};
export type useRemotePlayerUnregisterMutation = {
  response: useRemotePlayerUnregisterMutation$data;
  variables: useRemotePlayerUnregisterMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "id"
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
      }
    ],
    "kind": "ScalarField",
    "name": "unregisterPlayer",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "useRemotePlayerUnregisterMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "useRemotePlayerUnregisterMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "ad666f02adfe2f482937cf71089b7845",
    "id": null,
    "metadata": {},
    "name": "useRemotePlayerUnregisterMutation",
    "operationKind": "mutation",
    "text": "mutation useRemotePlayerUnregisterMutation(\n  $id: ID!\n) {\n  unregisterPlayer(id: $id)\n}\n"
  }
};
})();

(node as any).hash = "c857dbe93e81b231ce2f09f65b409db7";

export default node;
