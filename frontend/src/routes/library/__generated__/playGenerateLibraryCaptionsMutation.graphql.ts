/**
 * @generated SignedSource<<b7f1f634bb5cf7a921c6904e155444de>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type RecordingAutoCaptionsState = "FAILED" | "GENERATED" | "NOT_CONFIGURED" | "NOT_GENERATED" | "QUEUED" | "RUNNING" | "%future added value";
export type playGenerateLibraryCaptionsMutation$variables = {
  id: string;
};
export type playGenerateLibraryCaptionsMutation$data = {
  readonly generateLibraryCaptions: {
    readonly autoCaptionsStatus: {
      readonly count: number;
      readonly generatedAt: string | null | undefined;
      readonly model: string | null | undefined;
      readonly progress: {
        readonly ratio: number;
        readonly remainingSeconds: number | null | undefined;
      } | null | undefined;
      readonly state: RecordingAutoCaptionsState;
    };
    readonly id: string;
  };
};
export type playGenerateLibraryCaptionsMutation = {
  response: playGenerateLibraryCaptionsMutation$data;
  variables: playGenerateLibraryCaptionsMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "id"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "id",
        "variableName": "id"
      }
    ],
    "concreteType": "LibraryFile",
    "kind": "LinkedField",
    "name": "generateLibraryCaptions",
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
        "concreteType": "RecordingAutoCaptions",
        "kind": "LinkedField",
        "name": "autoCaptionsStatus",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "state",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "generatedAt",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "model",
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
    "name": "playGenerateLibraryCaptionsMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "playGenerateLibraryCaptionsMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "4e609b01a22a52db555fe0d39e184406",
    "id": null,
    "metadata": {},
    "name": "playGenerateLibraryCaptionsMutation",
    "operationKind": "mutation",
    "text": "mutation playGenerateLibraryCaptionsMutation(\n  $id: ID!\n) {\n  generateLibraryCaptions(id: $id) {\n    id\n    autoCaptionsStatus {\n      state\n      generatedAt\n      model\n      count\n      progress {\n        ratio\n        remainingSeconds\n      }\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "f8fb8bebff9f2f16488e44a17f372025";

export default node;
