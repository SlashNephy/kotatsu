/**
 * @generated SignedSource<<b658deba9ae6d174762137979a59fd38>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type CaptionModelKind = "VAD" | "WHISPER" | "%future added value";
export type CaptionModelState = "DOWNLOADING" | "FAILED" | "NOT_PLACED" | "PLACED" | "%future added value";
export type CaptionModelsCancelMutation$variables = {
  kind: CaptionModelKind;
};
export type CaptionModelsCancelMutation$data = {
  readonly cancelCaptionModelDownload: {
    readonly kind: CaptionModelKind;
    readonly state: CaptionModelState;
  };
};
export type CaptionModelsCancelMutation = {
  response: CaptionModelsCancelMutation$data;
  variables: CaptionModelsCancelMutation$variables;
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
    "name": "cancelCaptionModelDownload",
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
    "name": "CaptionModelsCancelMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "CaptionModelsCancelMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "82f0a49ea135685b14d94d4d2e9e9686",
    "id": null,
    "metadata": {},
    "name": "CaptionModelsCancelMutation",
    "operationKind": "mutation",
    "text": "mutation CaptionModelsCancelMutation(\n  $kind: CaptionModelKind!\n) {\n  cancelCaptionModelDownload(kind: $kind) {\n    kind\n    state\n  }\n}\n"
  }
};
})();

(node as any).hash = "251428b50519a68d2fb09449aa21c808";

export default node;
