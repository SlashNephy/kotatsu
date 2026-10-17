/**
 * @generated SignedSource<<ddf7935ae9fa4c03b4a07b1cf36068f0>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type RecordingIdReanalyzeMutation$variables = {
  id: string;
};
export type RecordingIdReanalyzeMutation$data = {
  readonly reanalyzeRecording: boolean;
};
export type RecordingIdReanalyzeMutation = {
  response: RecordingIdReanalyzeMutation$data;
  variables: RecordingIdReanalyzeMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "id"
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
      }
    ],
    "kind": "ScalarField",
    "name": "reanalyzeRecording",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "RecordingIdReanalyzeMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "RecordingIdReanalyzeMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "f697f948f16d444569968f9bd0976cd5",
    "id": null,
    "metadata": {},
    "name": "RecordingIdReanalyzeMutation",
    "operationKind": "mutation",
    "text": "mutation RecordingIdReanalyzeMutation(\n  $id: ID!\n) {\n  reanalyzeRecording(id: $id)\n}\n"
  }
};
})();

(node as any).hash = "6c6278e55e572e12c4b371887c472c1f";

export default node;
