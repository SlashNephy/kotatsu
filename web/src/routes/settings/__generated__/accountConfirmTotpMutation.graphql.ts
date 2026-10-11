/**
 * @generated SignedSource<<932ed9d652bb18e35536a7433db87fb6>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type accountConfirmTotpMutation$variables = {
  code: string;
};
export type accountConfirmTotpMutation$data = {
  readonly confirmTotpEnrollment: {
    readonly codes: ReadonlyArray<string>;
  };
};
export type accountConfirmTotpMutation = {
  response: accountConfirmTotpMutation$data;
  variables: accountConfirmTotpMutation$variables;
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
    "name": "accountConfirmTotpMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "accountConfirmTotpMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "beacddb887c1121c11070481eac6995d",
    "id": null,
    "metadata": {},
    "name": "accountConfirmTotpMutation",
    "operationKind": "mutation",
    "text": "mutation accountConfirmTotpMutation(\n  $code: String!\n) {\n  confirmTotpEnrollment(code: $code) {\n    codes\n  }\n}\n"
  }
};
})();

(node as any).hash = "f8a32c6b23922f4d08f3a72a9a8a4b96";

export default node;
