/**
 * @generated SignedSource<<244d2c6fd12216e050062000667032a6>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type AnimeFollowingViewingsRefreshMutation$variables = {
  tid: number;
};
export type AnimeFollowingViewingsRefreshMutation$data = {
  readonly refreshAnimeFollowingViewings: boolean;
};
export type AnimeFollowingViewingsRefreshMutation = {
  response: AnimeFollowingViewingsRefreshMutation$data;
  variables: AnimeFollowingViewingsRefreshMutation$variables;
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
    "kind": "ScalarField",
    "name": "refreshAnimeFollowingViewings",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "AnimeFollowingViewingsRefreshMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "AnimeFollowingViewingsRefreshMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "cc060fc71383afa3e746e7ae0be5468a",
    "id": null,
    "metadata": {},
    "name": "AnimeFollowingViewingsRefreshMutation",
    "operationKind": "mutation",
    "text": "mutation AnimeFollowingViewingsRefreshMutation(\n  $tid: Int!\n) {\n  refreshAnimeFollowingViewings(tid: $tid)\n}\n"
  }
};
})();

(node as any).hash = "221a8b856babc0e01eb5ad4932c6e082";

export default node;
