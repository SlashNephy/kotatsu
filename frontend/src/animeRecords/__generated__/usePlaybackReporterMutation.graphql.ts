/**
 * @generated SignedSource<<dcb23f1ead574639d9b0d4f9887a4eaa>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type PlaybackIntervalInput = {
  from: string;
  to: string;
};
export type usePlaybackReporterMutation$variables = {
  anime: boolean;
  drama: boolean;
  intervals: ReadonlyArray<PlaybackIntervalInput>;
  recordingId?: string | null | undefined;
  serviceId?: string | null | undefined;
};
export type usePlaybackReporterMutation$data = {
  readonly reportAnimePlayback: boolean;
};
export type usePlaybackReporterMutation = {
  response: usePlaybackReporterMutation$data;
  variables: usePlaybackReporterMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "anime"
},
v1 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "drama"
},
v2 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "intervals"
},
v3 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "recordingId"
},
v4 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "serviceId"
},
v5 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "anime",
        "variableName": "anime"
      },
      {
        "kind": "Variable",
        "name": "drama",
        "variableName": "drama"
      },
      {
        "kind": "Variable",
        "name": "intervals",
        "variableName": "intervals"
      },
      {
        "kind": "Variable",
        "name": "recordingId",
        "variableName": "recordingId"
      },
      {
        "kind": "Variable",
        "name": "serviceId",
        "variableName": "serviceId"
      }
    ],
    "kind": "ScalarField",
    "name": "reportAnimePlayback",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": [
      (v0/*:: as any*/),
      (v1/*:: as any*/),
      (v2/*:: as any*/),
      (v3/*:: as any*/),
      (v4/*:: as any*/)
    ],
    "kind": "Fragment",
    "metadata": null,
    "name": "usePlaybackReporterMutation",
    "selections": (v5/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [
      (v4/*:: as any*/),
      (v3/*:: as any*/),
      (v2/*:: as any*/),
      (v0/*:: as any*/),
      (v1/*:: as any*/)
    ],
    "kind": "Operation",
    "name": "usePlaybackReporterMutation",
    "selections": (v5/*:: as any*/)
  },
  "params": {
    "cacheID": "8e36192f44b6c746582b3df7e78b01ec",
    "id": null,
    "metadata": {},
    "name": "usePlaybackReporterMutation",
    "operationKind": "mutation",
    "text": "mutation usePlaybackReporterMutation(\n  $serviceId: ID\n  $recordingId: ID\n  $intervals: [PlaybackIntervalInput!]!\n  $anime: Boolean!\n  $drama: Boolean!\n) {\n  reportAnimePlayback(serviceId: $serviceId, recordingId: $recordingId, intervals: $intervals, anime: $anime, drama: $drama)\n}\n"
  }
};
})();

(node as any).hash = "9a655e5b0ca6569ce03101da3d5c57f3";

export default node;
