/**
 * @generated SignedSource<<8964cfea2e3c40070d2bcf2f5d43e929>>
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
  "name": "intervals"
},
v1 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "recordingId"
},
v2 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "serviceId"
},
v3 = [
  {
    "alias": null,
    "args": [
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
      (v2/*:: as any*/)
    ],
    "kind": "Fragment",
    "metadata": null,
    "name": "usePlaybackReporterMutation",
    "selections": (v3/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [
      (v2/*:: as any*/),
      (v1/*:: as any*/),
      (v0/*:: as any*/)
    ],
    "kind": "Operation",
    "name": "usePlaybackReporterMutation",
    "selections": (v3/*:: as any*/)
  },
  "params": {
    "cacheID": "893e8addeb5a87ebe55a313e2d7aa7c3",
    "id": null,
    "metadata": {},
    "name": "usePlaybackReporterMutation",
    "operationKind": "mutation",
    "text": "mutation usePlaybackReporterMutation(\n  $serviceId: ID\n  $recordingId: ID\n  $intervals: [PlaybackIntervalInput!]!\n) {\n  reportAnimePlayback(serviceId: $serviceId, recordingId: $recordingId, intervals: $intervals)\n}\n"
  }
};
})();

(node as any).hash = "30f098df25fe409d4692f6a8f1d1ded4";

export default node;
