/**
 * @generated SignedSource<<ce4257227825f6a90f79052b0026e191>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type RecordingAutoCaptionsState = "FAILED" | "GENERATED" | "NOT_CONFIGURED" | "NOT_GENERATED" | "QUEUED" | "RUNNING" | "%future added value";
export type libraryPlayGenerateCaptionsMutation$variables = {
  id: string;
};
export type libraryPlayGenerateCaptionsMutation$data = {
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
export type libraryPlayGenerateCaptionsMutation = {
  response: libraryPlayGenerateCaptionsMutation$data;
  variables: libraryPlayGenerateCaptionsMutation$variables;
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
    "name": "libraryPlayGenerateCaptionsMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "libraryPlayGenerateCaptionsMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "6514ac546cb014bd583cc4de3222e733",
    "id": null,
    "metadata": {},
    "name": "libraryPlayGenerateCaptionsMutation",
    "operationKind": "mutation",
    "text": "mutation libraryPlayGenerateCaptionsMutation(\n  $id: ID!\n) {\n  generateLibraryCaptions(id: $id) {\n    id\n    autoCaptionsStatus {\n      state\n      generatedAt\n      model\n      count\n      progress {\n        ratio\n        remainingSeconds\n      }\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "c764568fe00dfb3812c35c4b4e9bba7b";

export default node;
