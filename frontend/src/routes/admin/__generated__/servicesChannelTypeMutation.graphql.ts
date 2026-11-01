/**
 * @generated SignedSource<<e4150d0a8572c83a8c84cd24f9d0c1df>>
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
export type servicesChannelTypeMutation$variables = {
  input: ChannelTypeInput;
  type: string;
};
export type servicesChannelTypeMutation$data = {
  readonly updateChannelType: {
    readonly displayName: string;
    readonly sortOrder: number;
    readonly type: string;
  };
};
export type servicesChannelTypeMutation = {
  response: servicesChannelTypeMutation$data;
  variables: servicesChannelTypeMutation$variables;
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
    "name": "servicesChannelTypeMutation",
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
    "name": "servicesChannelTypeMutation",
    "selections": (v2/*:: as any*/)
  },
  "params": {
    "cacheID": "5882711f8cf1c88ca41b783a054816f2",
    "id": null,
    "metadata": {},
    "name": "servicesChannelTypeMutation",
    "operationKind": "mutation",
    "text": "mutation servicesChannelTypeMutation(\n  $type: String!\n  $input: ChannelTypeInput!\n) {\n  updateChannelType(type: $type, input: $input) {\n    type\n    displayName\n    sortOrder\n  }\n}\n"
  }
};
})();

(node as any).hash = "343e08e0d7796fa9f8739fdbbb441368";

export default node;
