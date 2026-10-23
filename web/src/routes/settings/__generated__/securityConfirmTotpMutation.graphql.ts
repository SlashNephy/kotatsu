/**
 * @generated SignedSource<<59a1e6072cc05724eba7813da608d5ee>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type securityConfirmTotpMutation$variables = {
  code: string;
};
export type securityConfirmTotpMutation$data = {
  readonly confirmTotpEnrollment: {
    readonly codes: ReadonlyArray<string>;
  };
};
export type securityConfirmTotpMutation = {
  response: securityConfirmTotpMutation$data;
  variables: securityConfirmTotpMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "code"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "code",
        "variableName": "code"
      }
    ],
    "concreteType": "RecoveryCodes",
    "kind": "LinkedField",
    "name": "confirmTotpEnrollment",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "codes",
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
    "name": "securityConfirmTotpMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "securityConfirmTotpMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "801d0c4f46902ce851603b11f79c63df",
    "id": null,
    "metadata": {},
    "name": "securityConfirmTotpMutation",
    "operationKind": "mutation",
    "text": "mutation securityConfirmTotpMutation(\n  $code: String!\n) {\n  confirmTotpEnrollment(code: $code) {\n    codes\n  }\n}\n"
  }
};
})();

(node as any).hash = "0b81c15db916d07ba9a2ef6476635a04";

export default node;
