/**
 * @generated SignedSource<<8a51c2a1a8ad69cf95608256418e2541>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type WatchPositionsClearMutation$variables = Record<PropertyKey, never>;
export type WatchPositionsClearMutation$data = {
  readonly clearAllWatchPositions: number;
};
export type WatchPositionsClearMutation = {
  response: WatchPositionsClearMutation$data;
  variables: WatchPositionsClearMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "clearAllWatchPositions",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "WatchPositionsClearMutation",
    "selections": (v0/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "WatchPositionsClearMutation",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "2fdf73d8c10bcabdb7fa4672569f4633",
    "id": null,
    "metadata": {},
    "name": "WatchPositionsClearMutation",
    "operationKind": "mutation",
    "text": "mutation WatchPositionsClearMutation {\n  clearAllWatchPositions\n}\n"
  }
};
})();

(node as any).hash = "a5db4b7971d4a13359168cdc1fbae9e5";

export default node;
