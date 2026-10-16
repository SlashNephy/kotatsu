/**
 * @generated SignedSource<<64519061baf88bd0a8d4a11f8d079e44>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type DataChangesViewerSettingsSubscription$variables = Record<PropertyKey, never>;
export type DataChangesViewerSettingsSubscription$data = {
  readonly viewerSettingsChanged: number;
};
export type DataChangesViewerSettingsSubscription = {
  response: DataChangesViewerSettingsSubscription$data;
  variables: DataChangesViewerSettingsSubscription$variables;
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
    "name": "DataChangesViewerSettingsSubscription",
    "selections": (v0/*:: as any*/),
    "type": "Subscription",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "DataChangesViewerSettingsSubscription",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "e58d8fdee1a9ab78e13f69a9d9cdc699",
    "id": null,
    "metadata": {},
    "name": "DataChangesViewerSettingsSubscription",
    "operationKind": "subscription",
    "text": "subscription DataChangesViewerSettingsSubscription {\n  viewerSettingsChanged\n}\n"
  }
};
})();

(node as any).hash = "2dff61d7558275190ef29dc1e9e840af";

export default node;
