/**
 * @generated SignedSource<<13738a04393cba509645dfdeb2a8a947>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type SkippedDuplicateListQuery$variables = Record<PropertyKey, never>;
export type SkippedDuplicateListQuery$data = {
  readonly skippedDuplicates: ReadonlyArray<{
    readonly lastRecordedAt: string;
    readonly program: {
      readonly endAt: string;
      readonly id: string;
      readonly name: string;
      readonly service: {
        readonly channelType: {
          readonly type: string;
        };
        readonly logoUrl: string | null | undefined;
        readonly name: string;
      };
      readonly startAt: string;
    };
    readonly rules: ReadonlyArray<{
      readonly id: string;
      readonly name: string;
    }>;
  }>;
};
export type SkippedDuplicateListQuery = {
  response: SkippedDuplicateListQuery$data;
  variables: SkippedDuplicateListQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v2 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "startAt",
  "storageKey": null
},
v3 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "endAt",
  "storageKey": null
},
v4 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "logoUrl",
  "storageKey": null
},
v5 = {
  "alias": null,
  "args": null,
  "concreteType": "ChannelType",
  "kind": "LinkedField",
  "name": "channelType",
  "plural": false,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "type",
      "storageKey": null
    }
  ],
  "storageKey": null
},
v6 = {
  "alias": null,
  "args": null,
  "concreteType": "SkippedRule",
  "kind": "LinkedField",
  "name": "rules",
  "plural": true,
  "selections": [
    (v0/*:: as any*/),
    (v1/*:: as any*/)
  ],
  "storageKey": null
},
v7 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "lastRecordedAt",
  "storageKey": null
};
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "SkippedDuplicateListQuery",
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "SkippedDuplicate",
        "kind": "LinkedField",
        "name": "skippedDuplicates",
        "plural": true,
        "selections": [
          {
            "alias": null,
            "args": null,
            "concreteType": "Program",
            "kind": "LinkedField",
            "name": "program",
            "plural": false,
            "selections": [
              (v0/*:: as any*/),
              (v1/*:: as any*/),
              (v2/*:: as any*/),
              (v3/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "Service",
                "kind": "LinkedField",
                "name": "service",
                "plural": false,
                "selections": [
                  (v1/*:: as any*/),
                  (v4/*:: as any*/),
                  (v5/*:: as any*/)
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          },
          (v6/*:: as any*/),
          (v7/*:: as any*/)
        ],
        "storageKey": null
      }
    ],
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "SkippedDuplicateListQuery",
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "SkippedDuplicate",
        "kind": "LinkedField",
        "name": "skippedDuplicates",
        "plural": true,
        "selections": [
          {
            "alias": null,
            "args": null,
            "concreteType": "Program",
            "kind": "LinkedField",
            "name": "program",
            "plural": false,
            "selections": [
              (v0/*:: as any*/),
              (v1/*:: as any*/),
              (v2/*:: as any*/),
              (v3/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "Service",
                "kind": "LinkedField",
                "name": "service",
                "plural": false,
                "selections": [
                  (v1/*:: as any*/),
                  (v4/*:: as any*/),
                  (v5/*:: as any*/),
                  (v0/*:: as any*/)
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          },
          (v6/*:: as any*/),
          (v7/*:: as any*/)
        ],
        "storageKey": null
      }
    ]
  },
  "params": {
    "cacheID": "df5138a3cbe0d9ec7459a60e57104a26",
    "id": null,
    "metadata": {},
    "name": "SkippedDuplicateListQuery",
    "operationKind": "query",
    "text": "query SkippedDuplicateListQuery {\n  skippedDuplicates {\n    program {\n      id\n      name\n      startAt\n      endAt\n      service {\n        name\n        logoUrl\n        channelType {\n          type\n        }\n        id\n      }\n    }\n    rules {\n      id\n      name\n    }\n    lastRecordedAt\n  }\n}\n"
  }
};
})();

(node as any).hash = "edfec17b45f68d73aa9deec97eda61ad";

export default node;
