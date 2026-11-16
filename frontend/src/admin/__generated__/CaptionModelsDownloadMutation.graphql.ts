/**
 * @generated SignedSource<<0049d4d8bceeaf5e6c08628eb6fe79c0>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type CaptionModelKind = "VAD" | "WHISPER" | "%future added value";
export type CaptionModelState = "DOWNLOADING" | "FAILED" | "NOT_PLACED" | "PLACED" | "%future added value";
export type CaptionModelsDownloadMutation$variables = {
  id: string;
};
export type CaptionModelsDownloadMutation$data = {
  readonly downloadCaptionModel: {
    readonly kind: CaptionModelKind;
    readonly state: CaptionModelState;
  };
};
export type CaptionModelsDownloadMutation = {
  response: CaptionModelsDownloadMutation$data;
  variables: CaptionModelsDownloadMutation$variables;
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
    "concreteType": "CaptionModelSlot",
    "kind": "LinkedField",
    "name": "downloadCaptionModel",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "kind",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "state",
        "storageKey": null
      }
    ],
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "CaptionModelsDownloadMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "CaptionModelsDownloadMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "8d4ea8524ef8f2a503b2163ff1648be2",
    "id": null,
    "metadata": {},
    "name": "CaptionModelsDownloadMutation",
    "operationKind": "mutation",
    "text": "mutation CaptionModelsDownloadMutation(\n  $id: String!\n) {\n  downloadCaptionModel(id: $id) {\n    kind\n    state\n  }\n}\n"
  }
};
})();

(node as any).hash = "3ecdc641663f740286638cfbcb34b3bf";

export default node;
