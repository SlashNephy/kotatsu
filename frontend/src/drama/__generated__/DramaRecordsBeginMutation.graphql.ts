/**
 * @generated SignedSource<<2670f9a430d78a1b78098fae4b7de07a>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type DramaRecordsBeginMutation$variables = Record<PropertyKey, never>;
export type DramaRecordsBeginMutation$data = {
  readonly beginDramaRecordConnection: string;
};
export type DramaRecordsBeginMutation = {
  response: DramaRecordsBeginMutation$data;
  variables: DramaRecordsBeginMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Literal",
        "name": "destination",
        "value": "SIMKL"
      }
    ],
    "kind": "ScalarField",
    "name": "beginDramaRecordConnection",
    "storageKey": "beginDramaRecordConnection(destination:\"SIMKL\")"
  }
];
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "DramaRecordsBeginMutation",
    "selections": (v0/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "DramaRecordsBeginMutation",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "caba3b38c87e29463041796a6300b3d3",
    "id": null,
    "metadata": {},
    "name": "DramaRecordsBeginMutation",
    "operationKind": "mutation",
    "text": "mutation DramaRecordsBeginMutation {\n  beginDramaRecordConnection(destination: SIMKL)\n}\n"
  }
};
})();

(node as any).hash = "aaef0e37e25c2e7d1e6e488c8e5e7bd0";

export default node;
