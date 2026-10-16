/**
 * @generated SignedSource<<ba42ed0e4f2dee543e2d22b32af12c31>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type RecordingIdRegenerateMutation$variables = {
  id: string;
};
export type RecordingIdRegenerateMutation$data = {
  readonly regenerateRecordingImages: boolean;
};
export type RecordingIdRegenerateMutation = {
  response: RecordingIdRegenerateMutation$data;
  variables: RecordingIdRegenerateMutation$variables;
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
    "name": "regenerateRecordingImages",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "RecordingIdRegenerateMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "RecordingIdRegenerateMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "f667c7798e238b4e2e8352aa99b8dc22",
    "id": null,
    "metadata": {},
    "name": "RecordingIdRegenerateMutation",
    "operationKind": "mutation",
    "text": "mutation RecordingIdRegenerateMutation(\n  $id: ID!\n) {\n  regenerateRecordingImages(id: $id)\n}\n"
  }
};
})();

(node as any).hash = "11e0f7d5d832296fe3a4d31524006343";

export default node;
