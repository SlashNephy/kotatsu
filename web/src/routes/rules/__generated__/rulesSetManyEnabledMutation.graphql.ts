/**
 * @generated SignedSource<<6c84f179445d7cf6c3615f18d931832d>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type rulesSetManyEnabledMutation$variables = {
  enabled: boolean;
  ids: ReadonlyArray<string>;
};
export type rulesSetManyEnabledMutation$data = {
  readonly setRulesEnabled: {
    readonly processed: number;
    readonly skipped: number;
  };
};
export type rulesSetManyEnabledMutation = {
  response: rulesSetManyEnabledMutation$data;
  variables: rulesSetManyEnabledMutation$variables;
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
  "name": "ids"
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
        "name": "ids",
        "variableName": "ids"
      }
    ],
    "concreteType": "BulkOperationPayload",
    "kind": "LinkedField",
    "name": "setRulesEnabled",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "processed",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "skipped",
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
    "name": "rulesSetManyEnabledMutation",
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
    "name": "rulesSetManyEnabledMutation",
    "selections": (v2/*:: as any*/)
  },
  "params": {
    "cacheID": "1a6ff5870672f6af76c4f743498f18f0",
    "id": null,
    "metadata": {},
    "name": "rulesSetManyEnabledMutation",
    "operationKind": "mutation",
    "text": "mutation rulesSetManyEnabledMutation(\n  $ids: [ID!]!\n  $enabled: Boolean!\n) {\n  setRulesEnabled(ids: $ids, enabled: $enabled) {\n    processed\n    skipped\n  }\n}\n"
  }
};
})();

(node as any).hash = "c6f78045c8b87736111f4b6737024eab";

export default node;
