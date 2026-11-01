/**
 * @generated SignedSource<<a7f93ec21bf96ccb5e286ec336e456bd>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ReaderFragment } from 'relay-runtime';
export type KeywordField = "DESCRIPTION" | "EXTENDED" | "NAME" | "%future added value";
export type KeywordMatch = "ALL" | "ANY" | "%future added value";
import { FragmentRefs } from "relay-runtime";
export type WatchRuleEditor_watchRule$data = {
  readonly conditions: {
    readonly channelTypes: ReadonlyArray<string>;
    readonly excludeGenres: ReadonlyArray<{
      readonly lv1: number;
      readonly lv2: number | null | undefined;
    }>;
    readonly excludeKeywords: ReadonlyArray<{
      readonly caseSensitive: boolean;
      readonly fields: ReadonlyArray<KeywordField>;
      readonly regexp: boolean;
      readonly text: string;
    }>;
    readonly excludeMarks: ReadonlyArray<string>;
    readonly excludeServices: ReadonlyArray<{
      readonly id: string;
    }>;
    readonly freeOnly: boolean;
    readonly genres: ReadonlyArray<{
      readonly lv1: number;
      readonly lv2: number | null | undefined;
    }>;
    readonly keywordMatch: KeywordMatch;
    readonly keywords: ReadonlyArray<{
      readonly caseSensitive: boolean;
      readonly fields: ReadonlyArray<KeywordField>;
      readonly regexp: boolean;
      readonly text: string;
    }>;
    readonly marks: ReadonlyArray<string>;
    readonly maxDurationSeconds: number | null | undefined;
    readonly minDurationSeconds: number | null | undefined;
    readonly services: ReadonlyArray<{
      readonly id: string;
    }>;
    readonly timeRanges: ReadonlyArray<{
      readonly durationMinutes: number;
      readonly startMinute: number;
      readonly weekdays: ReadonlyArray<number>;
    }>;
  };
  readonly enabled: boolean;
  readonly id: string;
  readonly name: string;
  readonly " $fragmentType": "WatchRuleEditor_watchRule";
};
export type WatchRuleEditor_watchRule$key = {
  readonly " $data"?: WatchRuleEditor_watchRule$data;
  readonly " $fragmentSpreads": FragmentRefs<"WatchRuleEditor_watchRule">;
};

const node: ReaderFragment = (function(){
var v0 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v1 = [
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
v2 = [
  (v0/*:: as any*/)
],
v3 = [
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
  "argumentDefinitions": [],
  "kind": "Fragment",
  "metadata": null,
  "name": "WatchRuleEditor_watchRule",
  "selections": [
    (v0/*:: as any*/),
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
          "selections": (v1/*:: as any*/),
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
          "selections": (v1/*:: as any*/),
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
          "selections": (v2/*:: as any*/),
          "storageKey": null
        },
        {
          "alias": null,
          "args": null,
          "concreteType": "Service",
          "kind": "LinkedField",
          "name": "excludeServices",
          "plural": true,
          "selections": (v2/*:: as any*/),
          "storageKey": null
        },
        {
          "alias": null,
          "args": null,
          "concreteType": "GenreCondition",
          "kind": "LinkedField",
          "name": "genres",
          "plural": true,
          "selections": (v3/*:: as any*/),
          "storageKey": null
        },
        {
          "alias": null,
          "args": null,
          "concreteType": "GenreCondition",
          "kind": "LinkedField",
          "name": "excludeGenres",
          "plural": true,
          "selections": (v3/*:: as any*/),
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
  "type": "WatchRule",
  "abstractKey": null
};
})();

(node as any).hash = "d6860b9b6df615da9a84328a1b487f81";

export default node;
