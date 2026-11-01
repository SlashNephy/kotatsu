/**
 * @generated SignedSource<<653ba966d5206f95e2d0caf8f3ec3250>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type WatchRuleListEnableMutation$variables = {
  enabled: boolean;
  id: string;
};
export type WatchRuleListEnableMutation$data = {
  readonly setWatchRuleEnabled: {
    readonly enabled: boolean;
    readonly id: string;
  };
};
export type WatchRuleListEnableMutation = {
  response: WatchRuleListEnableMutation$data;
  variables: WatchRuleListEnableMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "enabled"
},
v1 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "id"
},
v2 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "enabled",
        "variableName": "enabled"
      },
      {
        "kind": "Variable",
        "name": "id",
        "variableName": "id"
      }
    ],
    "concreteType": "WatchRule",
    "kind": "LinkedField",
    "name": "setWatchRuleEnabled",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "id",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "enabled",
        "storageKey": null
      }
    ],
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": [
      (v0/*:: as any*/),
      (v1/*:: as any*/)
    ],
    "kind": "Fragment",
    "metadata": null,
    "name": "WatchRuleListEnableMutation",
    "selections": (v2/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [
      (v1/*:: as any*/),
      (v0/*:: as any*/)
    ],
    "kind": "Operation",
    "name": "WatchRuleListEnableMutation",
    "selections": (v2/*:: as any*/)
  },
  "params": {
    "cacheID": "7a6360dffbe6f9d4323a98db2e47a9bb",
    "id": null,
    "metadata": {},
    "name": "WatchRuleListEnableMutation",
    "operationKind": "mutation",
    "text": "mutation WatchRuleListEnableMutation(\n  $id: ID!\n  $enabled: Boolean!\n) {\n  setWatchRuleEnabled(id: $id, enabled: $enabled) {\n    id\n    enabled\n  }\n}\n"
  }
};
})();

(node as any).hash = "9eaf6712968aec87c37bc9989f234685";

export default node;
