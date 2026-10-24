/**
 * @generated SignedSource<<ddff58481c9bae9dfadabcd2f6db0bee>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type PinButtonMutation$variables = {
  id: string;
  pinned: boolean;
};
export type PinButtonMutation$data = {
  readonly setServicePinned: ReadonlyArray<{
    readonly id: string;
  }>;
};
export type PinButtonMutation = {
  response: PinButtonMutation$data;
  variables: PinButtonMutation$variables;
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
    "name": "pinned"
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
        "name": "pinned",
        "variableName": "pinned"
      }
    ],
    "concreteType": "Service",
    "kind": "LinkedField",
    "name": "setServicePinned",
    "plural": true,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "id",
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
    "name": "PinButtonMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "PinButtonMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "3021fd0df3edb7c762607b12b32a8838",
    "id": null,
    "metadata": {},
    "name": "PinButtonMutation",
    "operationKind": "mutation",
    "text": "mutation PinButtonMutation(\n  $id: ID!\n  $pinned: Boolean!\n) {\n  setServicePinned(id: $id, pinned: $pinned) {\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "e61566b373ba5be9c1f55faf03d0d469";

export default node;
