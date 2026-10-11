/**
 * @generated SignedSource<<0662f60e3473b9dff7f3bd736488ae90>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type ChannelTypeInput = {
  displayName?: string | null | undefined;
  sortOrder?: number | null | undefined;
};
export type systemChannelTypeMutation$variables = {
  input: ChannelTypeInput;
  type: string;
};
export type systemChannelTypeMutation$data = {
  readonly updateChannelType: {
    readonly displayName: string;
    readonly sortOrder: number;
    readonly type: string;
  };
};
export type systemChannelTypeMutation = {
  response: systemChannelTypeMutation$data;
  variables: systemChannelTypeMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "input"
},
v1 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "type"
},
v2 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "input",
        "variableName": "input"
      },
      {
        "kind": "Variable",
        "name": "type",
        "variableName": "type"
      }
    ],
    "concreteType": "ChannelType",
    "kind": "LinkedField",
    "name": "updateChannelType",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "type",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "displayName",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "sortOrder",
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
    "name": "systemChannelTypeMutation",
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
    "name": "systemChannelTypeMutation",
    "selections": (v2/*:: as any*/)
  },
  "params": {
    "cacheID": "84e2f9396f8b053cea7eba6539541992",
    "id": null,
    "metadata": {},
    "name": "systemChannelTypeMutation",
    "operationKind": "mutation",
    "text": "mutation systemChannelTypeMutation(\n  $type: String!\n  $input: ChannelTypeInput!\n) {\n  updateChannelType(type: $type, input: $input) {\n    type\n    displayName\n    sortOrder\n  }\n}\n"
  }
};
})();

(node as any).hash = "92124afbcac1b26f35f7683e9758e3d4";

export default node;
