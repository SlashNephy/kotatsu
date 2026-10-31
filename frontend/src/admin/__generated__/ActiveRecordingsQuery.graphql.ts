/**
 * @generated SignedSource<<509b477c0de38201971563d53f3b9833>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type ActiveRecordingsQuery$variables = Record<PropertyKey, never>;
export type ActiveRecordingsQuery$data = {
  readonly recordings: {
    readonly edges: ReadonlyArray<{
      readonly node: {
        readonly drops: number;
        readonly errors: number;
        readonly id: string;
        readonly liveStats: {
          readonly bitrate: number;
          readonly drops: number;
          readonly errors: number;
          readonly scrambled: number;
        } | null | undefined;
        readonly name: string;
        readonly scrambled: number;
        readonly service: {
          readonly name: string;
        };
      };
    }>;
  };
};
export type ActiveRecordingsQuery = {
  response: ActiveRecordingsQuery$data;
  variables: ActiveRecordingsQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "kind": "Literal",
    "name": "filter",
    "value": {
      "states": [
        "RECORDING"
      ]
    }
  },
  {
    "kind": "Literal",
    "name": "first",
    "value": 50
  }
],
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v2 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v3 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "drops",
  "storageKey": null
},
v4 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "errors",
  "storageKey": null
},
v5 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "scrambled",
  "storageKey": null
},
v6 = {
  "alias": null,
  "args": null,
  "concreteType": "RecordingLiveStats",
  "kind": "LinkedField",
  "name": "liveStats",
  "plural": false,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "bitrate",
      "storageKey": null
    },
    (v3/*:: as any*/),
    (v4/*:: as any*/),
    (v5/*:: as any*/)
  ],
  "storageKey": null
};
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "ActiveRecordingsQuery",
    "selections": [
      {
        "alias": null,
        "args": (v0/*:: as any*/),
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
                  (v1/*:: as any*/),
                  (v2/*:: as any*/),
                  (v3/*:: as any*/),
                  (v4/*:: as any*/),
                  (v5/*:: as any*/),
                  {
                    "alias": null,
                    "args": null,
                    "concreteType": "Service",
                    "kind": "LinkedField",
                    "name": "service",
                    "plural": false,
                    "selections": [
                      (v2/*:: as any*/)
                    ],
                    "storageKey": null
                  },
                  (v6/*:: as any*/)
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          }
        ],
        "storageKey": "recordings(filter:{\"states\":[\"RECORDING\"]},first:50)"
      }
    ],
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "ActiveRecordingsQuery",
    "selections": [
      {
        "alias": null,
        "args": (v0/*:: as any*/),
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
                  (v1/*:: as any*/),
                  (v2/*:: as any*/),
                  (v3/*:: as any*/),
                  (v4/*:: as any*/),
                  (v5/*:: as any*/),
                  {
                    "alias": null,
                    "args": null,
                    "concreteType": "Service",
                    "kind": "LinkedField",
                    "name": "service",
                    "plural": false,
                    "selections": [
                      (v2/*:: as any*/),
                      (v1/*:: as any*/)
                    ],
                    "storageKey": null
                  },
                  (v6/*:: as any*/)
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          }
        ],
        "storageKey": "recordings(filter:{\"states\":[\"RECORDING\"]},first:50)"
      }
    ]
  },
  "params": {
    "cacheID": "4c3fa1f7f5378474f4b7822f4a9e7a25",
    "id": null,
    "metadata": {},
    "name": "ActiveRecordingsQuery",
    "operationKind": "query",
    "text": "query ActiveRecordingsQuery {\n  recordings(first: 50, filter: {states: [RECORDING]}) {\n    edges {\n      node {\n        id\n        name\n        drops\n        errors\n        scrambled\n        service {\n          name\n          id\n        }\n        liveStats {\n          bitrate\n          drops\n          errors\n          scrambled\n        }\n      }\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "b97333cbf5e46336d03c41c5185e29d8";

export default node;
