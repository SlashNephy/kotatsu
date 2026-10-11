/**
 * @generated SignedSource<<5eebdca7ed932779318e5aebde93fa26>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type accountRenamePasskeyMutation$variables = {
  id: string;
  name: string;
};
export type accountRenamePasskeyMutation$data = {
  readonly renamePasskey: {
    readonly id: string;
    readonly name: string;
  };
};
export type accountRenamePasskeyMutation = {
  response: accountRenamePasskeyMutation$data;
  variables: accountRenamePasskeyMutation$variables;
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
    "name": "accountRenamePasskeyMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "accountRenamePasskeyMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "7165c78aea992c4ce265ee993ca043b5",
    "id": null,
    "metadata": {},
    "name": "accountRenamePasskeyMutation",
    "operationKind": "mutation",
    "text": "mutation accountRenamePasskeyMutation(\n  $id: ID!\n  $name: String!\n) {\n  renamePasskey(id: $id, name: $name) {\n    id\n    name\n  }\n}\n"
  }
};
})();

(node as any).hash = "3c9db3c338864d8509186fb346e007f7";

export default node;
