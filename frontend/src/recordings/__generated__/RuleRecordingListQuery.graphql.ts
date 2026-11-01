/**
 * @generated SignedSource<<4e47b4a92068fed3f3d8bad9623ed299>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type RuleRecordingListQuery$variables = {
  first: number;
  ruleId: string;
};
export type RuleRecordingListQuery$data = {
  readonly recordings: {
    readonly edges: ReadonlyArray<{
      readonly node: {
        readonly id: string;
        readonly name: string;
        readonly startAt: string;
        readonly thumbnailUrl: string | null | undefined;
      };
    }>;
  };
};
export type RuleRecordingListQuery = {
  response: RuleRecordingListQuery$data;
  variables: RuleRecordingListQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "first"
},
v1 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "ruleId"
},
v2 = [
  {
    "alias": null,
    "args": [
      {
        "fields": [
          {
            "kind": "Variable",
            "name": "ruleId",
            "variableName": "ruleId"
          }
        ],
        "kind": "ObjectValue",
        "name": "filter"
      },
      {
        "kind": "Variable",
        "name": "first",
        "variableName": "first"
      }
    ],
    "concreteType": "RecordingConnection",
    "kind": "LinkedField",
    "name": "recordings",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "RecordingEdge",
        "kind": "LinkedField",
        "name": "edges",
        "plural": true,
        "selections": [
          {
            "alias": null,
            "args": null,
            "concreteType": "Recording",
            "kind": "LinkedField",
            "name": "node",
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
                "kind": "ScalarField",
                "name": "name",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "startAt",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "thumbnailUrl",
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
    "argumentDefinitions": [
      (v0/*:: as any*/),
      (v1/*:: as any*/)
    ],
    "kind": "Fragment",
    "metadata": null,
    "name": "RuleRecordingListQuery",
    "selections": (v2/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [
      (v1/*:: as any*/),
      (v0/*:: as any*/)
    ],
    "kind": "Operation",
    "name": "RuleRecordingListQuery",
    "selections": (v2/*:: as any*/)
  },
  "params": {
    "cacheID": "2aa5b66748f6f461be4899ac30cd221f",
    "id": null,
    "metadata": {},
    "name": "RuleRecordingListQuery",
    "operationKind": "query",
    "text": "query RuleRecordingListQuery(\n  $ruleId: ID!\n  $first: Int!\n) {\n  recordings(first: $first, filter: {ruleId: $ruleId}) {\n    edges {\n      node {\n        id\n        name\n        startAt\n        thumbnailUrl\n      }\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "ca3a954ea83552f6e9e2cf82b766db2f";

export default node;
