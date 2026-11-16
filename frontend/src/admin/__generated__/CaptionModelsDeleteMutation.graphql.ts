/**
 * @generated SignedSource<<38696ef8813150e87b140497f89bb485>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type CaptionModelKind = "VAD" | "WHISPER" | "%future added value";
export type CaptionModelState = "DOWNLOADING" | "FAILED" | "NOT_PLACED" | "PLACED" | "%future added value";
export type CaptionModelsDeleteMutation$variables = {
  kind: CaptionModelKind;
};
export type CaptionModelsDeleteMutation$data = {
  readonly deleteCaptionModel: {
    readonly kind: CaptionModelKind;
    readonly state: CaptionModelState;
  };
};
export type CaptionModelsDeleteMutation = {
  response: CaptionModelsDeleteMutation$data;
  variables: CaptionModelsDeleteMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "kind"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "kind",
        "variableName": "kind"
      }
    ],
    "concreteType": "CaptionModelSlot",
    "kind": "LinkedField",
    "name": "deleteCaptionModel",
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
    "name": "CaptionModelsDeleteMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "CaptionModelsDeleteMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "eafef41a2fb2f74740d636029f7d26a3",
    "id": null,
    "metadata": {},
    "name": "CaptionModelsDeleteMutation",
    "operationKind": "mutation",
    "text": "mutation CaptionModelsDeleteMutation(\n  $kind: CaptionModelKind!\n) {\n  deleteCaptionModel(kind: $kind) {\n    kind\n    state\n  }\n}\n"
  }
};
})();

(node as any).hash = "8468a0e49251b7abc5c283e052fd42b3";

export default node;
