/**
 * @generated SignedSource<<e87d47b8e6d42aec297634b63c9d22b9>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type RecordingIdGenerateCaptionsMutation$variables = {
  id: string;
};
export type RecordingIdGenerateCaptionsMutation$data = {
  readonly generateRecordingCaptions: boolean;
};
export type RecordingIdGenerateCaptionsMutation = {
  response: RecordingIdGenerateCaptionsMutation$data;
  variables: RecordingIdGenerateCaptionsMutation$variables;
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
    "name": "generateRecordingCaptions",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "RecordingIdGenerateCaptionsMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "RecordingIdGenerateCaptionsMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "cbdea850d81976327f85ece25fc2b555",
    "id": null,
    "metadata": {},
    "name": "RecordingIdGenerateCaptionsMutation",
    "operationKind": "mutation",
    "text": "mutation RecordingIdGenerateCaptionsMutation(\n  $id: ID!\n) {\n  generateRecordingCaptions(id: $id)\n}\n"
  }
};
})();

(node as any).hash = "01c871af563a60915da121c9c86a3106";

export default node;
