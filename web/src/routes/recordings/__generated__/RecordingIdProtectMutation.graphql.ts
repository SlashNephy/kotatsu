/**
 * @generated SignedSource<<9ae1b996e19b5962f800fc1871e38d44>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type RecordingIdProtectMutation$variables = {
  id: string;
  protected: boolean;
};
export type RecordingIdProtectMutation$data = {
  readonly setRecordingProtected: {
    readonly id: string;
    readonly protected: boolean;
  };
};
export type RecordingIdProtectMutation = {
  response: RecordingIdProtectMutation$data;
  variables: RecordingIdProtectMutation$variables;
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
    "name": "protected"
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
        "name": "protected",
        "variableName": "protected"
      }
    ],
    "concreteType": "Recording",
    "kind": "LinkedField",
    "name": "setRecordingProtected",
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
        "name": "protected",
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
    "name": "RecordingIdProtectMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "RecordingIdProtectMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "e7d6c27718bea97b2b756387c3fbc8a4",
    "id": null,
    "metadata": {},
    "name": "RecordingIdProtectMutation",
    "operationKind": "mutation",
    "text": "mutation RecordingIdProtectMutation(\n  $id: ID!\n  $protected: Boolean!\n) {\n  setRecordingProtected(id: $id, protected: $protected) {\n    id\n    protected\n  }\n}\n"
  }
};
})();

(node as any).hash = "b89bec1dbb6735962491bc83e579194a";

export default node;
