/**
 * @generated SignedSource<<805106d341aa1f6dc5fd7646f25dec4a>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type DramaLibraryImportMutation$variables = Record<PropertyKey, never>;
export type DramaLibraryImportMutation$data = {
  readonly importDramaWatchlist: boolean;
};
export type DramaLibraryImportMutation = {
  response: DramaLibraryImportMutation$data;
  variables: DramaLibraryImportMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "importDramaWatchlist",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "DramaLibraryImportMutation",
    "selections": (v0/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "DramaLibraryImportMutation",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "c4e6d672f0f537ed647c8ed7e9400dc8",
    "id": null,
    "metadata": {},
    "name": "DramaLibraryImportMutation",
    "operationKind": "mutation",
    "text": "mutation DramaLibraryImportMutation {\n  importDramaWatchlist\n}\n"
  }
};
})();

(node as any).hash = "a0aaab92e288eba6f0aaee827ff64ac8";

export default node;
