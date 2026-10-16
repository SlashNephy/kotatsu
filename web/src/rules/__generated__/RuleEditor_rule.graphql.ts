/**
 * @generated SignedSource<<dbd1bb24e3ade1659e761484145280e2>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ReaderFragment } from 'relay-runtime';
export type KeywordField = "DESCRIPTION" | "EXTENDED" | "NAME" | "%future added value";
export type KeywordMatch = "ALL" | "ANY" | "%future added value";
import { FragmentRefs } from "relay-runtime";
export type RuleEditor_rule$data = {
  readonly avoidDuplicates: boolean;
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
  readonly duplicateWindowDays: number | null | undefined;
  readonly enabled: boolean;
  readonly followEventRelay: boolean;
  readonly id: string;
  readonly memo: string;
  readonly name: string;
  readonly settings: {
    readonly endMarginSeconds: number | null | undefined;
    readonly fileNameTemplate: string | null | undefined;
    readonly joinSegments: boolean | null | undefined;
    readonly poolId: string | null | undefined;
    readonly priority: number | null | undefined;
    readonly startMarginSeconds: number | null | undefined;
  };
  readonly validFrom: string | null | undefined;
  readonly validUntil: string | null | undefined;
  readonly " $fragmentType": "RuleEditor_rule";
};
export type RuleEditor_rule$key = {
  readonly " $data"?: RuleEditor_rule$data;
  readonly " $fragmentSpreads": FragmentRefs<"RuleEditor_rule">;
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
  "name": "RuleEditor_rule",
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
      "name": "memo",
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
        }
      ],
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "concreteType": "RecordingSettingsOverrides",
      "kind": "LinkedField",
      "name": "settings",
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
      "kind": "ScalarField",
      "name": "validFrom",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "validUntil",
      "storageKey": null
    },
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
  "type": "Rule",
  "abstractKey": null
};
})();

(node as any).hash = "05aa60ac07afe9ddb1794932e8df1a44";

export default node;
