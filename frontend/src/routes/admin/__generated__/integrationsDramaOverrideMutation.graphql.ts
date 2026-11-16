/**
 * @generated SignedSource<<bf49e6bb37e15f734a470886fccf7ffa>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type DramaTitleOverrideInput = {
  exclude?: boolean;
  programName?: string | null | undefined;
  reset?: boolean;
  titleKey?: string | null | undefined;
  tmdbSeriesId?: number | null | undefined;
};
export type integrationsDramaOverrideMutation$variables = {
  input: DramaTitleOverrideInput;
};
export type integrationsDramaOverrideMutation$data = {
  readonly updateDramaTitleOverride: ReadonlyArray<{
    readonly titleKey: string;
  }>;
};
export type integrationsDramaOverrideMutation = {
  response: integrationsDramaOverrideMutation$data;
  variables: integrationsDramaOverrideMutation$variables;
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
    "concreteType": "DramaTitleOverride",
    "kind": "LinkedField",
    "name": "updateDramaTitleOverride",
    "plural": true,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "titleKey",
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
    "name": "integrationsDramaOverrideMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "integrationsDramaOverrideMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "8c7afb2197b60346efc4a6ef22e577e6",
    "id": null,
    "metadata": {},
    "name": "integrationsDramaOverrideMutation",
    "operationKind": "mutation",
    "text": "mutation integrationsDramaOverrideMutation(\n  $input: DramaTitleOverrideInput!\n) {\n  updateDramaTitleOverride(input: $input) {\n    titleKey\n  }\n}\n"
  }
};
})();

(node as any).hash = "1ba8495f17dd3300d3cab689968726f3";

export default node;
