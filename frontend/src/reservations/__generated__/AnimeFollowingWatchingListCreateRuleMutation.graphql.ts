/**
 * @generated SignedSource<<db79bbb160fc5a390b6789284e05044e>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type AnimeFollowingWatchingListCreateRuleMutation$variables = {
  tid: number;
};
export type AnimeFollowingWatchingListCreateRuleMutation$data = {
  readonly createAnimeFollowingWatchingRule: {
    readonly id: string;
    readonly name: string;
  };
};
export type AnimeFollowingWatchingListCreateRuleMutation = {
  response: AnimeFollowingWatchingListCreateRuleMutation$data;
  variables: AnimeFollowingWatchingListCreateRuleMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "tid"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "tid",
        "variableName": "tid"
      }
    ],
    "concreteType": "Rule",
    "kind": "LinkedField",
    "name": "createAnimeFollowingWatchingRule",
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
        "name": "name",
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
    "name": "AnimeFollowingWatchingListCreateRuleMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "AnimeFollowingWatchingListCreateRuleMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "86700ebaca343028899193aee39377f0",
    "id": null,
    "metadata": {},
    "name": "AnimeFollowingWatchingListCreateRuleMutation",
    "operationKind": "mutation",
    "text": "mutation AnimeFollowingWatchingListCreateRuleMutation(\n  $tid: Int!\n) {\n  createAnimeFollowingWatchingRule(tid: $tid) {\n    id\n    name\n  }\n}\n"
  }
};
})();

(node as any).hash = "0663af15bbfe02e884640d11ace2249a";

export default node;
