/**
 * @generated SignedSource<<1daad27656ba60c39fdb866c351b7bd9>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type deviceApproveMutation$variables = {
  userCode: string;
};
export type deviceApproveMutation$data = {
  readonly approveDeviceAuthorization: {
    readonly approved: boolean;
  };
};
export type deviceApproveMutation = {
  response: deviceApproveMutation$data;
  variables: deviceApproveMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "userCode"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "userCode",
        "variableName": "userCode"
      }
    ],
    "concreteType": "DeviceAuthorization",
    "kind": "LinkedField",
    "name": "approveDeviceAuthorization",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "approved",
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
    "name": "deviceApproveMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "deviceApproveMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "43fa8d51e32c268f4e5dbcb494efb38f",
    "id": null,
    "metadata": {},
    "name": "deviceApproveMutation",
    "operationKind": "mutation",
    "text": "mutation deviceApproveMutation(\n  $userCode: String!\n) {\n  approveDeviceAuthorization(userCode: $userCode) {\n    approved\n  }\n}\n"
  }
};
})();

(node as any).hash = "19d80a51422e3dd4b5580db889e22b7d";

export default node;
