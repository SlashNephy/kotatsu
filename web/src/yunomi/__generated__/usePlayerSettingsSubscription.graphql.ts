/**
 * @generated SignedSource<<25991984b7357f1e1d994bc5f86b369a>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type usePlayerSettingsSubscription$variables = Record<PropertyKey, never>;
export type usePlayerSettingsSubscription$data = {
  readonly viewerSettingsChanged: number;
};
export type usePlayerSettingsSubscription = {
  response: usePlayerSettingsSubscription$data;
  variables: usePlayerSettingsSubscription$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "viewerSettingsChanged",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "usePlayerSettingsSubscription",
    "selections": (v0/*:: as any*/),
    "type": "Subscription",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "usePlayerSettingsSubscription",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "6b74f427c1ddf156bb45b2b7bc95f914",
    "id": null,
    "metadata": {},
    "name": "usePlayerSettingsSubscription",
    "operationKind": "subscription",
    "text": "subscription usePlayerSettingsSubscription {\n  viewerSettingsChanged\n}\n"
  }
};
})();

(node as any).hash = "07dd2931047b7fcb346c5d7f38f1b33e";

export default node;
