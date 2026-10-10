/**
 * @generated SignedSource<<fe72017941e3a3a1d7394b96b5ddf149>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type GuideViewInput = {
  excludeGenres?: ReadonlyArray<GenreInput> | null | undefined;
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
export type GuideViewEditorCreateMutation$variables = {
  input: GuideViewInput;
};
export type GuideViewEditorCreateMutation$data = {
  readonly createGuideView: {
    readonly id: string;
  };
};
export type GuideViewEditorCreateMutation = {
  response: GuideViewEditorCreateMutation$data;
  variables: GuideViewEditorCreateMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
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
        "name": "input",
        "variableName": "input"
      }
    ],
    "concreteType": "GuideView",
    "kind": "LinkedField",
    "name": "createGuideView",
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
    "name": "GuideViewEditorCreateMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "GuideViewEditorCreateMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "7694302256d0e71861c9c4919729a8d5",
    "id": null,
    "metadata": {},
    "name": "GuideViewEditorCreateMutation",
    "operationKind": "mutation",
    "text": "mutation GuideViewEditorCreateMutation(\n  $input: GuideViewInput!\n) {\n  createGuideView(input: $input) {\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "fa50546deae1de25adebe04b2142c744";

export default node;
