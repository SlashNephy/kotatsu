/**
 * @generated SignedSource<<db2088178efb52f36233b86625df3cd9>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
import { FragmentRefs } from "relay-runtime";
export type KeywordField = "DESCRIPTION" | "EXTENDED" | "NAME" | "%future added value";
export type KeywordMatch = "ALL" | "ANY" | "%future added value";
export type WatchRuleInput = {
  conditions: ProgramConditionsInput;
  enabled?: boolean;
  name: string;
};
export type ProgramConditionsInput = {
  channelTypes?: ReadonlyArray<string> | null | undefined;
  excludeGenres?: ReadonlyArray<GenreInput> | null | undefined;
  excludeKeywords?: ReadonlyArray<KeywordInput> | null | undefined;
  excludeMarks?: ReadonlyArray<string> | null | undefined;
  excludeServices?: ReadonlyArray<string> | null | undefined;
  freeOnly?: boolean | null | undefined;
  genres?: ReadonlyArray<GenreInput> | null | undefined;
  keywordMatch?: KeywordMatch;
  keywords?: ReadonlyArray<KeywordInput> | null | undefined;
  marks?: ReadonlyArray<string> | null | undefined;
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
export type WatchRuleEditorUpdateMutation$variables = {
  id: string;
  input: WatchRuleInput;
};
export type WatchRuleEditorUpdateMutation$data = {
  readonly updateWatchRule: {
    readonly " $fragmentSpreads": FragmentRefs<"WatchRuleEditor_watchRule">;
  };
};
export type WatchRuleEditorUpdateMutation = {
  response: WatchRuleEditorUpdateMutation$data;
  variables: WatchRuleEditorUpdateMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "id"
  },
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "input"
  }
],
v1 = [
  {
    "kind": "Variable",
    "name": "id",
    "variableName": "id"
  },
  {
    "kind": "Variable",
    "name": "input",
    "variableName": "input"
  }
],
v2 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v3 = [
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "text",
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "regexp",
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "caseSensitive",
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "fields",
    "storageKey": null
  }
],
v4 = [
  (v2/*:: as any*/)
],
v5 = [
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "lv1",
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "lv2",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "WatchRuleEditorUpdateMutation",
    "selections": [
      {
        "alias": null,
        "args": (v1/*:: as any*/),
        "concreteType": "WatchRule",
        "kind": "LinkedField",
        "name": "updateWatchRule",
        "plural": false,
        "selections": [
          {
            "args": null,
            "kind": "FragmentSpread",
            "name": "WatchRuleEditor_watchRule"
          }
        ],
        "storageKey": null
      }
    ],
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "WatchRuleEditorUpdateMutation",
    "selections": [
      {
        "alias": null,
        "args": (v1/*:: as any*/),
        "concreteType": "WatchRule",
        "kind": "LinkedField",
        "name": "updateWatchRule",
        "plural": false,
        "selections": [
          (v2/*:: as any*/),
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
            "name": "enabled",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "concreteType": "ProgramConditions",
            "kind": "LinkedField",
            "name": "conditions",
            "plural": false,
            "selections": [
              {
                "alias": null,
                "args": null,
                "concreteType": "Keyword",
                "kind": "LinkedField",
                "name": "keywords",
                "plural": true,
                "selections": (v3/*:: as any*/),
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "keywordMatch",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "concreteType": "Keyword",
                "kind": "LinkedField",
                "name": "excludeKeywords",
                "plural": true,
                "selections": (v3/*:: as any*/),
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "channelTypes",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "concreteType": "Service",
                "kind": "LinkedField",
                "name": "services",
                "plural": true,
                "selections": (v4/*:: as any*/),
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "concreteType": "Service",
                "kind": "LinkedField",
                "name": "excludeServices",
                "plural": true,
                "selections": (v4/*:: as any*/),
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "concreteType": "GenreCondition",
                "kind": "LinkedField",
                "name": "genres",
                "plural": true,
                "selections": (v5/*:: as any*/),
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "concreteType": "GenreCondition",
                "kind": "LinkedField",
                "name": "excludeGenres",
                "plural": true,
                "selections": (v5/*:: as any*/),
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "concreteType": "TimeRange",
                "kind": "LinkedField",
                "name": "timeRanges",
                "plural": true,
                "selections": [
                  {
                    "alias": null,
                    "args": null,
                    "kind": "ScalarField",
                    "name": "weekdays",
                    "storageKey": null
                  },
                  {
                    "alias": null,
                    "args": null,
                    "kind": "ScalarField",
                    "name": "startMinute",
                    "storageKey": null
                  },
                  {
                    "alias": null,
                    "args": null,
                    "kind": "ScalarField",
                    "name": "durationMinutes",
                    "storageKey": null
                  }
                ],
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "minDurationSeconds",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "maxDurationSeconds",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "freeOnly",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "marks",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "excludeMarks",
                "storageKey": null
              }
            ],
            "storageKey": null
          }
        ],
        "storageKey": null
      }
    ]
  },
  "params": {
    "cacheID": "c655c875c1e2c1dcb6faa3ca60a9cc1f",
    "id": null,
    "metadata": {},
    "name": "WatchRuleEditorUpdateMutation",
    "operationKind": "mutation",
    "text": "mutation WatchRuleEditorUpdateMutation(\n  $id: ID!\n  $input: WatchRuleInput!\n) {\n  updateWatchRule(id: $id, input: $input) {\n    ...WatchRuleEditor_watchRule\n    id\n  }\n}\n\nfragment WatchRuleEditor_watchRule on WatchRule {\n  id\n  name\n  enabled\n  conditions {\n    keywords {\n      text\n      regexp\n      caseSensitive\n      fields\n    }\n    keywordMatch\n    excludeKeywords {\n      text\n      regexp\n      caseSensitive\n      fields\n    }\n    channelTypes\n    services {\n      id\n    }\n    excludeServices {\n      id\n    }\n    genres {\n      lv1\n      lv2\n    }\n    excludeGenres {\n      lv1\n      lv2\n    }\n    timeRanges {\n      weekdays\n      startMinute\n      durationMinutes\n    }\n    minDurationSeconds\n    maxDurationSeconds\n    freeOnly\n    marks\n    excludeMarks\n  }\n}\n"
  }
};
})();

(node as any).hash = "5c45c3e08f5eec700f2f19806bb2c549";

export default node;
