/**
 * @generated SignedSource<<16a4b49ea29e2c4b2091c05c318920af>>
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
    readonly excludeGenres: ReadonlyArray<{
      readonly lv1: number;
      readonly lv2: number | null | undefined;
    }>;
    readonly freeOnly: boolean;
    readonly genres: ReadonlyArray<{
      readonly lv1: number;
      readonly lv2: number | null | undefined;
    }>;
    readonly hideEmptyServices: boolean;
    readonly id: string;
    readonly minDurationSeconds: number | null | undefined;
    readonly name: string;
    readonly services: ReadonlyArray<{
      readonly id: string;
    }>;
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
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v2 = [
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "lv1",
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "lv2",
    "storageKey": null
  }
],
v3 = [
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
      (v1/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "name",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "GenreCondition",
        "kind": "LinkedField",
        "name": "genres",
        "plural": true,
        "selections": (v2/*:: as any*/),
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "GenreCondition",
        "kind": "LinkedField",
        "name": "excludeGenres",
        "plural": true,
        "selections": (v2/*:: as any*/),
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "minDurationSeconds",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "Service",
        "kind": "LinkedField",
        "name": "services",
        "plural": true,
        "selections": [
          (v1/*:: as any*/)
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "hideEmptyServices",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "freeOnly",
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
    "selections": (v3/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "GuideViewEditorUpdateMutation",
    "selections": (v3/*:: as any*/)
  },
  "params": {
    "cacheID": "198c5a497340abba366778f8ec630845",
    "id": null,
    "metadata": {},
    "name": "GuideViewEditorUpdateMutation",
    "operationKind": "mutation",
    "text": "mutation GuideViewEditorUpdateMutation(\n  $id: ID!\n  $input: GuideViewInput!\n) {\n  updateGuideView(id: $id, input: $input) {\n    id\n    name\n    genres {\n      lv1\n      lv2\n    }\n    excludeGenres {\n      lv1\n      lv2\n    }\n    minDurationSeconds\n    services {\n      id\n    }\n    hideEmptyServices\n    freeOnly\n  }\n}\n"
  }
};
})();

(node as any).hash = "f2363300d676262eb4429183734cbaf3";

export default node;
