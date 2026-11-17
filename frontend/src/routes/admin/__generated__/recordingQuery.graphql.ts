/**
 * @generated SignedSource<<4879c27cdfb312078ab0bec36dcf9e76>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type CMDetector = "FFMPEG" | "LOGO" | "%future added value";
export type recordingQuery$variables = Record<PropertyKey, never>;
export type recordingQuery$data = {
  readonly systemSettings: {
    readonly captionThreads: number;
    readonly cmDetector: CMDetector;
    readonly recordingDefaults: {
      readonly detectCm: boolean;
      readonly endMarginSeconds: number;
      readonly fileNameTemplate: string;
      readonly generateCaptions: boolean;
      readonly joinSegments: boolean;
      readonly poolId: string | null | undefined;
      readonly priority: number;
      readonly startMarginSeconds: number;
      readonly transcodeFrameRate: string;
      readonly transcodePreset: string | null | undefined;
    };
    readonly ruleDefaults: {
      readonly avoidDuplicates: boolean;
      readonly bundleSameEpisodes: boolean;
      readonly duplicateWindowDays: number | null | undefined;
      readonly followEventRelay: boolean;
    };
    readonly ruleUnmatchedGraceMinutes: number;
    readonly sleepControlEnabled: boolean;
    readonly thumbnailPreviewsEnabled: boolean;
    readonly tvtplayChaptersEnabled: boolean;
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
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "detectCm",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "generateCaptions",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "transcodePreset",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "transcodeFrameRate",
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
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "bundleSameEpisodes",
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "ruleUnmatchedGraceMinutes",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "tvtplayChaptersEnabled",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "cmDetector",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "thumbnailPreviewsEnabled",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "captionThreads",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "sleepControlEnabled",
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
    "cacheID": "32da381fc586570a73d4d3592aa54887",
    "id": null,
    "metadata": {},
    "name": "recordingQuery",
    "operationKind": "query",
    "text": "query recordingQuery {\n  systemSettings {\n    recordingDefaults {\n      poolId\n      fileNameTemplate\n      startMarginSeconds\n      endMarginSeconds\n      priority\n      joinSegments\n      detectCm\n      generateCaptions\n      transcodePreset\n      transcodeFrameRate\n    }\n    ruleDefaults {\n      avoidDuplicates\n      duplicateWindowDays\n      followEventRelay\n      bundleSameEpisodes\n    }\n    ruleUnmatchedGraceMinutes\n    tvtplayChaptersEnabled\n    cmDetector\n    thumbnailPreviewsEnabled\n    captionThreads\n    sleepControlEnabled\n    updatedAt\n  }\n  systemStatus {\n    pools {\n      id\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "bedf0d15f68e6880ad8a222d0c3c64ae";

export default node;
