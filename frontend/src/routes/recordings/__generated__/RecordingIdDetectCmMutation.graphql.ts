/**
 * @generated SignedSource<<34e5e2d456559148f4404a5dc6464848>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type RecordingIdDetectCmMutation$variables = {
  id: string;
};
export type RecordingIdDetectCmMutation$data = {
  readonly detectRecordingCm: boolean;
};
export type RecordingIdDetectCmMutation = {
  response: RecordingIdDetectCmMutation$data;
  variables: RecordingIdDetectCmMutation$variables;
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
    "name": "detectRecordingCm",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "RecordingIdDetectCmMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "RecordingIdDetectCmMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "01d11c6fcc2ae12d40aab7591099ff24",
    "id": null,
    "metadata": {},
    "name": "RecordingIdDetectCmMutation",
    "operationKind": "mutation",
    "text": "mutation RecordingIdDetectCmMutation(\n  $id: ID!\n) {\n  detectRecordingCm(id: $id)\n}\n"
  }
};
})();

(node as any).hash = "720449932722eed535ca4585949329dc";

export default node;
