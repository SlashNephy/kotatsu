/**
 * @generated SignedSource<<5f896dc3d547039778c5078eb59bf5b5>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type recordingQuery$variables = Record<PropertyKey, never>;
export type recordingQuery$data = {
  readonly systemSettings: {
    readonly recordingDefaults: {
      readonly endMarginSeconds: number;
      readonly fileNameTemplate: string;
      readonly joinSegments: boolean;
      readonly poolId: string | null | undefined;
      readonly priority: number;
      readonly startMarginSeconds: number;
    };
    readonly ruleDefaults: {
      readonly avoidDuplicates: boolean;
      readonly duplicateWindowDays: number | null | undefined;
      readonly followEventRelay: boolean;
    };
    readonly updatedAt: string | null | undefined;
  };
  readonly systemStatus: {
    readonly pools: ReadonlyArray<{
      readonly id: string;
    }>;
  };
};
export type recordingQuery = {
  response: recordingQuery$data;
  variables: recordingQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "SystemSettings",
    "kind": "LinkedField",
    "name": "systemSettings",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "RecordingSettings",
        "kind": "LinkedField",
        "name": "recordingDefaults",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "poolId",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "fileNameTemplate",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "startMarginSeconds",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "endMarginSeconds",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "priority",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "joinSegments",
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "RuleDefaults",
        "kind": "LinkedField",
        "name": "ruleDefaults",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "avoidDuplicates",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "duplicateWindowDays",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "followEventRelay",
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "updatedAt",
        "storageKey": null
      }
    ],
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "concreteType": "SystemStatus",
    "kind": "LinkedField",
    "name": "systemStatus",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "StoragePool",
        "kind": "LinkedField",
        "name": "pools",
        "plural": true,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "id",
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
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "recordingQuery",
    "selections": (v0/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "recordingQuery",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "ea91e61d68b651c6cd5f673abee77cab",
    "id": null,
    "metadata": {},
    "name": "recordingQuery",
    "operationKind": "query",
    "text": "query recordingQuery {\n  systemSettings {\n    recordingDefaults {\n      poolId\n      fileNameTemplate\n      startMarginSeconds\n      endMarginSeconds\n      priority\n      joinSegments\n    }\n    ruleDefaults {\n      avoidDuplicates\n      duplicateWindowDays\n      followEventRelay\n    }\n    updatedAt\n  }\n  systemStatus {\n    pools {\n      id\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "6ad55e5baf7488c5abfb678f5a76b7d3";

export default node;
