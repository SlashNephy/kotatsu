/**
 * @generated SignedSource<<ff4a80d964ea2c39d20230a5b6c7e99e>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type RecordingFailureReason = "CANCELLED" | "PROGRAM_NOT_FOUND" | "RECORDER_UNAVAILABLE" | "STORAGE_FULL" | "STORAGE_UNAVAILABLE" | "STREAM_INTERRUPTED" | "TUNER_SERVER_UNREACHABLE" | "TUNER_UNAVAILABLE" | "%future added value";
export type FailureCountsResetMutation$variables = {
  reasons: ReadonlyArray<RecordingFailureReason>;
};
export type FailureCountsResetMutation$data = {
  readonly resetRecordingFailureCounts: ReadonlyArray<{
    readonly count: number;
    readonly lastFailedAt: string | null | undefined;
    readonly reason: RecordingFailureReason;
    readonly resetAt: string | null | undefined;
  }>;
};
export type FailureCountsResetMutation = {
  response: FailureCountsResetMutation$data;
  variables: FailureCountsResetMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "reasons"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "reasons",
        "variableName": "reasons"
      }
    ],
    "concreteType": "RecordingFailureCount",
    "kind": "LinkedField",
    "name": "resetRecordingFailureCounts",
    "plural": true,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "reason",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "count",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "lastFailedAt",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "resetAt",
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
    "name": "FailureCountsResetMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "FailureCountsResetMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "2a467de820d0ee22be4084cf131e70a3",
    "id": null,
    "metadata": {},
    "name": "FailureCountsResetMutation",
    "operationKind": "mutation",
    "text": "mutation FailureCountsResetMutation(\n  $reasons: [RecordingFailureReason!]!\n) {\n  resetRecordingFailureCounts(reasons: $reasons) {\n    reason\n    count\n    lastFailedAt\n    resetAt\n  }\n}\n"
  }
};
})();

(node as any).hash = "32e670527f19cb1379f9fea2b77260b9";

export default node;
