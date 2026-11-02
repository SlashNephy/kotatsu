/**
 * @generated SignedSource<<64decdfbbd210120d3300d17d8342b8b>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type RecordAnimeEpisodesInput = {
  episode?: number | null | undefined;
  program?: string | null | undefined;
  recording?: string | null | undefined;
  tid?: number | null | undefined;
  title?: string | null | undefined;
};
export type RecordAnimeButtonMutation$variables = {
  input: RecordAnimeEpisodesInput;
};
export type RecordAnimeButtonMutation$data = {
  readonly recordAnimeEpisodes: boolean;
};
export type RecordAnimeButtonMutation = {
  response: RecordAnimeButtonMutation$data;
  variables: RecordAnimeButtonMutation$variables;
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
    "kind": "ScalarField",
    "name": "recordAnimeEpisodes",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "RecordAnimeButtonMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "RecordAnimeButtonMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "2fa4e33460d6a4d47a28f8a819901629",
    "id": null,
    "metadata": {},
    "name": "RecordAnimeButtonMutation",
    "operationKind": "mutation",
    "text": "mutation RecordAnimeButtonMutation(\n  $input: RecordAnimeEpisodesInput!\n) {\n  recordAnimeEpisodes(input: $input)\n}\n"
  }
};
})();

(node as any).hash = "34932c58dfc3c3fc9d2af8cd8e91c1c1";

export default node;
