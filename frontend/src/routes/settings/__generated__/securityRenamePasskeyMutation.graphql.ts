/**
 * @generated SignedSource<<1524df827069ec55714a184853ad182e>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type securityRenamePasskeyMutation$variables = {
  id: string;
  name: string;
};
export type securityRenamePasskeyMutation$data = {
  readonly renamePasskey: {
    readonly id: string;
    readonly name: string;
  };
};
export type securityRenamePasskeyMutation = {
  response: securityRenamePasskeyMutation$data;
  variables: securityRenamePasskeyMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "id"
  },
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "name"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "id",
        "variableName": "id"
      },
      {
        "kind": "Variable",
        "name": "name",
        "variableName": "name"
      }
    ],
    "concreteType": "Passkey",
    "kind": "LinkedField",
    "name": "renamePasskey",
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
        "name": "name",
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
    "name": "securityRenamePasskeyMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "securityRenamePasskeyMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "f160936820016fa9ae32c5366df5ae39",
    "id": null,
    "metadata": {},
    "name": "securityRenamePasskeyMutation",
    "operationKind": "mutation",
    "text": "mutation securityRenamePasskeyMutation(\n  $id: ID!\n  $name: String!\n) {\n  renamePasskey(id: $id, name: $name) {\n    id\n    name\n  }\n}\n"
  }
};
})();

(node as any).hash = "fdeb1c1fb742bfced4dd535feb92b059";

export default node;
