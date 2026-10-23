/**
 * @generated SignedSource<<e3b1a82fdc60eb31dfb4ffbba711b5cd>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type rulesEnableMutation$variables = {
  enabled: boolean;
  id: string;
};
export type rulesEnableMutation$data = {
  readonly setRuleEnabled: {
    readonly enabled: boolean;
    readonly id: string;
  };
};
export type rulesEnableMutation = {
  response: rulesEnableMutation$data;
  variables: rulesEnableMutation$variables;
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
    "concreteType": "Rule",
    "kind": "LinkedField",
    "name": "setRuleEnabled",
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
    "name": "rulesEnableMutation",
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
    "name": "rulesEnableMutation",
    "selections": (v2/*:: as any*/)
  },
  "params": {
    "cacheID": "2b47a77bb09644791005d092c4598077",
    "id": null,
    "metadata": {},
    "name": "rulesEnableMutation",
    "operationKind": "mutation",
    "text": "mutation rulesEnableMutation(\n  $id: ID!\n  $enabled: Boolean!\n) {\n  setRuleEnabled(id: $id, enabled: $enabled) {\n    id\n    enabled\n  }\n}\n"
  }
};
})();

(node as any).hash = "a0c60adbbd697d17d6796a011d3cc8ca";

export default node;
