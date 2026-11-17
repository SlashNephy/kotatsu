/**
 * @generated SignedSource<<e868ef007f4d9a200a8c29d9c2d47bf2>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type DramaWatchControlsWatchedMutation$variables = {
  episodeNumber: number;
  seasonNumber: number;
  seriesId: number;
  watched: boolean;
};
export type DramaWatchControlsWatchedMutation$data = {
  readonly setDramaEpisodeWatched: boolean;
};
export type DramaWatchControlsWatchedMutation = {
  response: DramaWatchControlsWatchedMutation$data;
  variables: DramaWatchControlsWatchedMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "episodeNumber"
},
v1 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "seasonNumber"
},
v2 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "seriesId"
},
v3 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "watched"
},
v4 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "episodeNumber",
        "variableName": "episodeNumber"
      },
      {
        "kind": "Variable",
        "name": "seasonNumber",
        "variableName": "seasonNumber"
      },
      {
        "kind": "Variable",
        "name": "seriesId",
        "variableName": "seriesId"
      },
      {
        "kind": "Variable",
        "name": "watched",
        "variableName": "watched"
      }
    ],
    "kind": "ScalarField",
    "name": "setDramaEpisodeWatched",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": [
      (v0/*:: as any*/),
      (v1/*:: as any*/),
      (v2/*:: as any*/),
      (v3/*:: as any*/)
    ],
    "kind": "Fragment",
    "metadata": null,
    "name": "DramaWatchControlsWatchedMutation",
    "selections": (v4/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [
      (v2/*:: as any*/),
      (v1/*:: as any*/),
      (v0/*:: as any*/),
      (v3/*:: as any*/)
    ],
    "kind": "Operation",
    "name": "DramaWatchControlsWatchedMutation",
    "selections": (v4/*:: as any*/)
  },
  "params": {
    "cacheID": "c67deaf1cb544c65bd656cee0d9f1331",
    "id": null,
    "metadata": {},
    "name": "DramaWatchControlsWatchedMutation",
    "operationKind": "mutation",
    "text": "mutation DramaWatchControlsWatchedMutation(\n  $seriesId: Int!\n  $seasonNumber: Int!\n  $episodeNumber: Int!\n  $watched: Boolean!\n) {\n  setDramaEpisodeWatched(seriesId: $seriesId, seasonNumber: $seasonNumber, episodeNumber: $episodeNumber, watched: $watched)\n}\n"
  }
};
})();

(node as any).hash = "37f61b5ee379601495cfafc0f1279d53";

export default node;
