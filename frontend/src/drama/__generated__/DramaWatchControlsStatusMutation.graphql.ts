/**
 * @generated SignedSource<<c50ca74a455d1b1a5c7d7d7dd1d6185b>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type DramaWatchStatus = "WANNA_WATCH" | "WATCHING" | "%future added value";
export type DramaWatchControlsStatusMutation$variables = {
  seriesId: number;
  status?: DramaWatchStatus | null | undefined;
};
export type DramaWatchControlsStatusMutation$data = {
  readonly setDramaWatchStatus: {
    readonly tmdbId: number;
    readonly watchStatus: DramaWatchStatus | null | undefined;
  };
};
export type DramaWatchControlsStatusMutation = {
  response: DramaWatchControlsStatusMutation$data;
  variables: DramaWatchControlsStatusMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "seriesId"
  },
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "status"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "seriesId",
        "variableName": "seriesId"
      },
      {
        "kind": "Variable",
        "name": "status",
        "variableName": "status"
      }
    ],
    "concreteType": "DramaSeries",
    "kind": "LinkedField",
    "name": "setDramaWatchStatus",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "tmdbId",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "watchStatus",
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
    "name": "DramaWatchControlsStatusMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "DramaWatchControlsStatusMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "02928a39c515d853f7a7f74304031c67",
    "id": null,
    "metadata": {},
    "name": "DramaWatchControlsStatusMutation",
    "operationKind": "mutation",
    "text": "mutation DramaWatchControlsStatusMutation(\n  $seriesId: Int!\n  $status: DramaWatchStatus\n) {\n  setDramaWatchStatus(seriesId: $seriesId, status: $status) {\n    tmdbId\n    watchStatus\n  }\n}\n"
  }
};
})();

(node as any).hash = "df9c91c27a968601c009e3b81ccd52b4";

export default node;
