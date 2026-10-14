/**
 * @generated SignedSource<<2ccf1c3c65946348fb1fff68109877d7>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type KeywordField = "DESCRIPTION" | "EXTENDED" | "NAME" | "%future added value";
export type ProgramConditionsInput = {
  channelTypes?: ReadonlyArray<string> | null | undefined;
  excludeGenres?: ReadonlyArray<GenreInput> | null | undefined;
  excludeKeywords?: ReadonlyArray<KeywordInput> | null | undefined;
  freeOnly?: boolean | null | undefined;
  genres?: ReadonlyArray<GenreInput> | null | undefined;
  keywords?: ReadonlyArray<KeywordInput> | null | undefined;
  maxDurationSeconds?: number | null | undefined;
  minDurationSeconds?: number | null | undefined;
  services?: ReadonlyArray<string> | null | undefined;
  timeRanges?: ReadonlyArray<TimeRangeInput> | null | undefined;
};
export type KeywordInput = {
  caseSensitive?: boolean;
  fields: ReadonlyArray<KeywordField>;
  regexp?: boolean;
  text: string;
};
export type GenreInput = {
  lv1: number;
  lv2?: number | null | undefined;
};
export type TimeRangeInput = {
  durationMinutes: number;
  startMinute: number;
  weekdays: ReadonlyArray<number>;
};
export type ArchiveRangeInput = {
  from?: string | null | undefined;
  to?: string | null | undefined;
};
export type searchProgramsQuery$variables = {
  after?: string | null | undefined;
  archive?: ArchiveRangeInput | null | undefined;
  conditions: ProgramConditionsInput;
  first: number;
};
export type searchProgramsQuery$data = {
  readonly searchPrograms: {
    readonly edges: ReadonlyArray<{
      readonly node: {
        readonly archived: boolean;
        readonly id: string;
        readonly name: string;
        readonly service: {
          readonly logoUrl: string | null | undefined;
          readonly name: string;
        };
        readonly startAt: string;
      };
    }>;
    readonly pageInfo: {
      readonly endCursor: string | null | undefined;
      readonly hasNextPage: boolean;
    };
  };
};
export type searchProgramsQuery = {
  response: searchProgramsQuery$data;
  variables: searchProgramsQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "after"
},
v1 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "archive"
},
v2 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "conditions"
},
v3 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "first"
},
v4 = [
  {
    "kind": "Variable",
    "name": "after",
    "variableName": "after"
  },
  {
    "kind": "Variable",
    "name": "archive",
    "variableName": "archive"
  },
  {
    "kind": "Variable",
    "name": "conditions",
    "variableName": "conditions"
  },
  {
    "kind": "Variable",
    "name": "first",
    "variableName": "first"
  }
],
v5 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v6 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v7 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "startAt",
  "storageKey": null
},
v8 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "archived",
  "storageKey": null
},
v9 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "logoUrl",
  "storageKey": null
},
v10 = {
  "alias": null,
  "args": null,
  "concreteType": "PageInfo",
  "kind": "LinkedField",
  "name": "pageInfo",
  "plural": false,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "hasNextPage",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "endCursor",
      "storageKey": null
    }
  ],
  "storageKey": null
};
return {
  "fragment": {
    "argumentDefinitions": [
      (v0/*:: as any*/),
      (v1/*:: as any*/),
      (v2/*:: as any*/),
      (v3/*:: as any*/)
    ],
    "kind": "Fragment",
    "metadata": null,
    "name": "searchProgramsQuery",
    "selections": [
      {
        "alias": null,
        "args": (v4/*:: as any*/),
        "concreteType": "ProgramConnection",
        "kind": "LinkedField",
        "name": "searchPrograms",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "concreteType": "ProgramEdge",
            "kind": "LinkedField",
            "name": "edges",
            "plural": true,
            "selections": [
              {
                "alias": null,
                "args": null,
                "concreteType": "Program",
                "kind": "LinkedField",
                "name": "node",
                "plural": false,
                "selections": [
                  (v5/*:: as any*/),
                  (v6/*:: as any*/),
                  (v7/*:: as any*/),
                  (v8/*:: as any*/),
                  {
                    "alias": null,
                    "args": null,
                    "concreteType": "Service",
                    "kind": "LinkedField",
                    "name": "service",
                    "plural": false,
                    "selections": [
                      (v6/*:: as any*/),
                      (v9/*:: as any*/)
                    ],
                    "storageKey": null
                  }
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          },
          (v10/*:: as any*/)
        ],
        "storageKey": null
      }
    ],
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [
      (v2/*:: as any*/),
      (v1/*:: as any*/),
      (v3/*:: as any*/),
      (v0/*:: as any*/)
    ],
    "kind": "Operation",
    "name": "searchProgramsQuery",
    "selections": [
      {
        "alias": null,
        "args": (v4/*:: as any*/),
        "concreteType": "ProgramConnection",
        "kind": "LinkedField",
        "name": "searchPrograms",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "concreteType": "ProgramEdge",
            "kind": "LinkedField",
            "name": "edges",
            "plural": true,
            "selections": [
              {
                "alias": null,
                "args": null,
                "concreteType": "Program",
                "kind": "LinkedField",
                "name": "node",
                "plural": false,
                "selections": [
                  (v5/*:: as any*/),
                  (v6/*:: as any*/),
                  (v7/*:: as any*/),
                  (v8/*:: as any*/),
                  {
                    "alias": null,
                    "args": null,
                    "concreteType": "Service",
                    "kind": "LinkedField",
                    "name": "service",
                    "plural": false,
                    "selections": [
                      (v6/*:: as any*/),
                      (v9/*:: as any*/),
                      (v5/*:: as any*/)
                    ],
                    "storageKey": null
                  }
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          },
          (v10/*:: as any*/)
        ],
        "storageKey": null
      }
    ]
  },
  "params": {
    "cacheID": "fa15f2ceb4141f59127cd9c28ac10478",
    "id": null,
    "metadata": {},
    "name": "searchProgramsQuery",
    "operationKind": "query",
    "text": "query searchProgramsQuery(\n  $conditions: ProgramConditionsInput!\n  $archive: ArchiveRangeInput\n  $first: Int!\n  $after: String\n) {\n  searchPrograms(conditions: $conditions, archive: $archive, first: $first, after: $after) {\n    edges {\n      node {\n        id\n        name\n        startAt\n        archived\n        service {\n          name\n          logoUrl\n          id\n        }\n      }\n    }\n    pageInfo {\n      hasNextPage\n      endCursor\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "325ded2ab9da15e17f5558c2c3bacdea";

export default node;
