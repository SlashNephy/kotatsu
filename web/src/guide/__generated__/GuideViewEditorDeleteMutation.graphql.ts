/**
 * @generated SignedSource<<3f76005bbf87342b2c7b126a18c5303b>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type GuideViewEditorDeleteMutation$variables = {
  id: string;
};
export type GuideViewEditorDeleteMutation$data = {
  readonly deleteGuideView: string;
};
export type GuideViewEditorDeleteMutation = {
  response: GuideViewEditorDeleteMutation$data;
  variables: GuideViewEditorDeleteMutation$variables;
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
    "name": "deleteGuideView",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "GuideViewEditorDeleteMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "GuideViewEditorDeleteMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "0fa5bdf91a77d425c2ef7a639a7ce321",
    "id": null,
    "metadata": {},
    "name": "GuideViewEditorDeleteMutation",
    "operationKind": "mutation",
    "text": "mutation GuideViewEditorDeleteMutation(\n  $id: ID!\n) {\n  deleteGuideView(id: $id)\n}\n"
  }
};
})();

(node as any).hash = "c9db1560152175c3b109408f832d2cbb";

export default node;
