/**
 * @generated SignedSource<<50d23b08913245e4fdd9edaeaa7530af>>
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
export type DramaEpisodeOverrideMutation$variables = {
  input: DramaTitleOverrideInput;
};
export type DramaEpisodeOverrideMutation$data = {
  readonly updateDramaTitleOverride: ReadonlyArray<{
    readonly titleKey: string;
  }>;
};
export type DramaEpisodeOverrideMutation = {
  response: DramaEpisodeOverrideMutation$data;
  variables: DramaEpisodeOverrideMutation$variables;
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
    "name": "DramaEpisodeOverrideMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "DramaEpisodeOverrideMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "fef6b6738e98049da33789db14c22310",
    "id": null,
    "metadata": {},
    "name": "DramaEpisodeOverrideMutation",
    "operationKind": "mutation",
    "text": "mutation DramaEpisodeOverrideMutation(\n  $input: DramaTitleOverrideInput!\n) {\n  updateDramaTitleOverride(input: $input) {\n    titleKey\n  }\n}\n"
  }
};
})();

(node as any).hash = "7c6d8b344ac3d7ce9bf1092cc13c4d26";

export default node;
