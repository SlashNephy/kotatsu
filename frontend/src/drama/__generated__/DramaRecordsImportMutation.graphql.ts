/**
 * @generated SignedSource<<5349597cb25d72c1ba315926f5832bfb>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type DramaRecordsImportMutation$variables = Record<PropertyKey, never>;
export type DramaRecordsImportMutation$data = {
  readonly importDramaWatchlist: boolean;
};
export type DramaRecordsImportMutation = {
  response: DramaRecordsImportMutation$data;
  variables: DramaRecordsImportMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Literal",
        "name": "force",
        "value": true
      }
    ],
    "kind": "ScalarField",
    "name": "importDramaWatchlist",
    "storageKey": "importDramaWatchlist(force:true)"
  }
];
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "DramaRecordsImportMutation",
    "selections": (v0/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "DramaRecordsImportMutation",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "33f73ed1d7194b505ba930b8489f29b4",
    "id": null,
    "metadata": {},
    "name": "DramaRecordsImportMutation",
    "operationKind": "mutation",
    "text": "mutation DramaRecordsImportMutation {\n  importDramaWatchlist(force: true)\n}\n"
  }
};
})();

(node as any).hash = "a4f5b099afec93e6390f63ea20edc67e";

export default node;
