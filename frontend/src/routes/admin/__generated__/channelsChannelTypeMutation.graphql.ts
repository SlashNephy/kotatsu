/**
 * @generated SignedSource<<a8fc7d0121bbcfa7679cde26595ccb99>>
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
export type channelsChannelTypeMutation$variables = {
  input: ChannelTypeInput;
  type: string;
};
export type channelsChannelTypeMutation$data = {
  readonly updateChannelType: {
    readonly displayName: string;
    readonly sortOrder: number;
    readonly type: string;
  };
};
export type channelsChannelTypeMutation = {
  response: channelsChannelTypeMutation$data;
  variables: channelsChannelTypeMutation$variables;
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
    "name": "channelsChannelTypeMutation",
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
    "name": "channelsChannelTypeMutation",
    "selections": (v2/*:: as any*/)
  },
  "params": {
    "cacheID": "c73803f68a3a9a5e8d2cfe6494a27165",
    "id": null,
    "metadata": {},
    "name": "channelsChannelTypeMutation",
    "operationKind": "mutation",
    "text": "mutation channelsChannelTypeMutation(\n  $type: String!\n  $input: ChannelTypeInput!\n) {\n  updateChannelType(type: $type, input: $input) {\n    type\n    displayName\n    sortOrder\n  }\n}\n"
  }
};
})();

(node as any).hash = "e7d1ae73848d56ff46548d8f9ce19be0";

export default node;
