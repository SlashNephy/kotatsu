/**
 * @generated SignedSource<<7429cafd903ec1c19ae8345ebe26fc34>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type JobState = "AVAILABLE" | "CANCELLED" | "COMPLETED" | "DISCARDED" | "PENDING" | "RETRYABLE" | "RUNNING" | "SCHEDULED" | "%future added value";
export type jobsQuery$variables = {
  state: JobState;
};
export type jobsQuery$data = {
  readonly jobCounts: ReadonlyArray<{
    readonly count: number;
    readonly state: JobState;
  }>;
  readonly jobs: {
    readonly nodes: ReadonlyArray<{
      readonly attempt: number;
      readonly attemptedAt: string | null | undefined;
      readonly cancellable: boolean;
      readonly createdAt: string;
      readonly errors: ReadonlyArray<{
        readonly at: string;
        readonly attempt: number;
        readonly message: string;
      }>;
      readonly finalizedAt: string | null | undefined;
      readonly id: string;
      readonly kind: string;
      readonly maxAttempts: number;
      readonly progress: {
        readonly ratio: number;
        readonly remainingSeconds: number | null | undefined;
      } | null | undefined;
      readonly queue: string;
      readonly recording: {
        readonly id: string;
        readonly name: string;
      } | null | undefined;
      readonly retryable: boolean;
      readonly scheduledAt: string;
      readonly state: JobState;
    }>;
  };
};
export type jobsQuery = {
  response: jobsQuery$data;
  variables: jobsQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "state"
  }
],
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "state",
  "storageKey": null
},
v2 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v3 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "attempt",
  "storageKey": null
},
v4 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "JobStateCount",
    "kind": "LinkedField",
    "name": "jobCounts",
    "plural": true,
    "selections": [
      (v1/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "count",
        "storageKey": null
      }
    ],
    "storageKey": null
  },
  {
    "alias": null,
    "args": [
      {
        "kind": "Literal",
        "name": "first",
        "value": 100
      },
      {
        "kind": "Variable",
        "name": "state",
        "variableName": "state"
      }
    ],
    "concreteType": "JobPage",
    "kind": "LinkedField",
    "name": "jobs",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "Job",
        "kind": "LinkedField",
        "name": "nodes",
        "plural": true,
        "selections": [
          (v2/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "kind",
            "storageKey": null
          },
          (v1/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "queue",
            "storageKey": null
          },
          (v3/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "maxAttempts",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "createdAt",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "scheduledAt",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "attemptedAt",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "finalizedAt",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "retryable",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "cancellable",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "concreteType": "JobError",
            "kind": "LinkedField",
            "name": "errors",
            "plural": true,
            "selections": [
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "at",
                "storageKey": null
              },
              (v3/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "message",
                "storageKey": null
              }
            ],
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "concreteType": "Recording",
            "kind": "LinkedField",
            "name": "recording",
            "plural": false,
            "selections": [
              (v2/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "name",
                "storageKey": null
              }
            ],
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "concreteType": "JobProgress",
            "kind": "LinkedField",
            "name": "progress",
            "plural": false,
            "selections": [
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "ratio",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "remainingSeconds",
                "storageKey": null
              }
            ],
            "storageKey": null
          }
        ],
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
    "name": "jobsQuery",
    "selections": (v4/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "jobsQuery",
    "selections": (v4/*:: as any*/)
  },
  "params": {
    "cacheID": "2a6bc710e1542232b96cf84e1d2884c1",
    "id": null,
    "metadata": {},
    "name": "jobsQuery",
    "operationKind": "query",
    "text": "query jobsQuery(\n  $state: JobState!\n) {\n  jobCounts {\n    state\n    count\n  }\n  jobs(state: $state, first: 100) {\n    nodes {\n      id\n      kind\n      state\n      queue\n      attempt\n      maxAttempts\n      createdAt\n      scheduledAt\n      attemptedAt\n      finalizedAt\n      retryable\n      cancellable\n      errors {\n        at\n        attempt\n        message\n      }\n      recording {\n        id\n        name\n      }\n      progress {\n        ratio\n        remainingSeconds\n      }\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "bc77319f39683dead8b4632d571c0030";

export default node;
