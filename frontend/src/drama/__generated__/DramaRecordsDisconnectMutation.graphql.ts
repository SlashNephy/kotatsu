/**
 * @generated SignedSource<<82b7b8ef950af6a9e036d43812262604>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type DramaRecordsDisconnectMutation$variables = Record<PropertyKey, never>;
export type DramaRecordsDisconnectMutation$data = {
  readonly disconnectDramaRecord: {
    readonly enabled: boolean;
  };
};
export type DramaRecordsDisconnectMutation = {
  response: DramaRecordsDisconnectMutation$data;
  variables: DramaRecordsDisconnectMutation$variables;
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
    "concreteType": "DramaRecordSettings",
    "kind": "LinkedField",
    "name": "disconnectDramaRecord",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "enabled",
        "storageKey": null
      }
    ],
    "storageKey": "disconnectDramaRecord(destination:\"SIMKL\")"
  }
];
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "DramaRecordsDisconnectMutation",
    "selections": (v0/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "DramaRecordsDisconnectMutation",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "a63ecaac22088db5680549c97ae12cb7",
    "id": null,
    "metadata": {},
    "name": "DramaRecordsDisconnectMutation",
    "operationKind": "mutation",
    "text": "mutation DramaRecordsDisconnectMutation {\n  disconnectDramaRecord(destination: SIMKL) {\n    enabled\n  }\n}\n"
  }
};
})();

(node as any).hash = "8ca35322e52216a6571583eec10da603";

export default node;
