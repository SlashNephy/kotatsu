/**
 * @generated SignedSource<<d89a6f5b75715d4a645fc42170ed8b08>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type GuideViewInput = {
  excludeGenres?: ReadonlyArray<GenreInput> | null | undefined;
  freeOnly?: boolean;
  genres?: ReadonlyArray<GenreInput> | null | undefined;
  hideEmptyServices?: boolean;
  minDurationSeconds?: number | null | undefined;
  name: string;
  services?: ReadonlyArray<string> | null | undefined;
};
export type GenreInput = {
  lv1: number;
  lv2?: number | null | undefined;
};
export type GuideViewEditorUpdateMutation$variables = {
  id: string;
  input: GuideViewInput;
};
export type GuideViewEditorUpdateMutation$data = {
  readonly updateGuideView: {
    readonly id: string;
  };
};
export type GuideViewEditorUpdateMutation = {
  response: GuideViewEditorUpdateMutation$data;
  variables: GuideViewEditorUpdateMutation$variables;
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
    "name": "input"
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
        "name": "input",
        "variableName": "input"
      }
    ],
    "concreteType": "GuideView",
    "kind": "LinkedField",
    "name": "updateGuideView",
    "plural": false,
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
    "name": "GuideViewEditorUpdateMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "GuideViewEditorUpdateMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "2a37b12cb92fa0f77f2dcc344806cad1",
    "id": null,
    "metadata": {},
    "name": "GuideViewEditorUpdateMutation",
    "operationKind": "mutation",
    "text": "mutation GuideViewEditorUpdateMutation(\n  $id: ID!\n  $input: GuideViewInput!\n) {\n  updateGuideView(id: $id, input: $input) {\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "aa0d8df0bbdcd78e1ccb732ae6cc04aa";

export default node;
