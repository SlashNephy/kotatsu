/**
 * @generated SignedSource<<f77442eb34e2039e87bd3843569987b8>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type accountRegenerateMutation$variables = Record<PropertyKey, never>;
export type accountRegenerateMutation$data = {
  readonly regenerateRecoveryCodes: {
    readonly codes: ReadonlyArray<string>;
  };
};
export type accountRegenerateMutation = {
  response: accountRegenerateMutation$data;
  variables: accountRegenerateMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "RecoveryCodes",
    "kind": "LinkedField",
    "name": "regenerateRecoveryCodes",
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
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "accountRegenerateMutation",
    "selections": (v0/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "accountRegenerateMutation",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "1118382eb349bcfa7ea957f1f8088235",
    "id": null,
    "metadata": {},
    "name": "accountRegenerateMutation",
    "operationKind": "mutation",
    "text": "mutation accountRegenerateMutation {\n  regenerateRecoveryCodes {\n    codes\n  }\n}\n"
  }
};
})();

(node as any).hash = "3f6efe9c6de7b7db989c257ef50c6bfb";

export default node;
