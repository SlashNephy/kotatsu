/**
 * @generated SignedSource<<6402aef3e1a2088f135482422bfa715e>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type RecordingIdDeleteMutation$variables = {
  id: string;
};
export type RecordingIdDeleteMutation$data = {
  readonly deleteRecording: string;
};
export type RecordingIdDeleteMutation = {
  response: RecordingIdDeleteMutation$data;
  variables: RecordingIdDeleteMutation$variables;
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
    "name": "deleteRecording",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "RecordingIdDeleteMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "RecordingIdDeleteMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "9615408ee434b127576a904314c627ef",
    "id": null,
    "metadata": {},
    "name": "RecordingIdDeleteMutation",
    "operationKind": "mutation",
    "text": "mutation RecordingIdDeleteMutation(\n  $id: ID!\n) {\n  deleteRecording(id: $id)\n}\n"
  }
};
})();

(node as any).hash = "af9e0a797e321a932874c8b2b206a8c8";

export default node;
