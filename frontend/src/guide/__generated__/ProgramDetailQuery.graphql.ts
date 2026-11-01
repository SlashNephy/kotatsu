/**
 * @generated SignedSource<<1d9a0ce7a1b135f25d093a9af0e6fc28>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type AssignmentState = "CONFLICT" | "OK" | "PARTIAL" | "PENDING" | "%future added value";
export type ReservationKind = "PROGRAM" | "RULE" | "TIME" | "%future added value";
export type Role = "ADMIN" | "MEMBER" | "VIEWER" | "%future added value";
export type ProgramDetailQuery$variables = {
  id: string;
};
export type ProgramDetailQuery$data = {
  readonly node: {
    readonly __typename: "Program";
    readonly animeEpisodes: ReadonlyArray<{
      readonly countEnd: number | null | undefined;
      readonly countStart: number | null | undefined;
      readonly first: boolean;
      readonly last: boolean;
      readonly subtitle: string | null | undefined;
      readonly syobocalUrl: string | null | undefined;
      readonly works: ReadonlyArray<{
        readonly anilistUrl: string | null | undefined;
        readonly annictUrl: string | null | undefined;
        readonly malUrl: string | null | undefined;
      }>;
    }>;
    readonly archived: boolean;
    readonly description: string;
    readonly endAt: string;
    readonly endUndecided: boolean;
    readonly eventId: number;
    readonly extended: ReadonlyArray<{
      readonly heading: string;
      readonly text: string;
    }>;
    readonly genres: ReadonlyArray<{
      readonly lv1: number;
      readonly lv2: number;
    }>;
    readonly id: string;
    readonly isFree: boolean;
    readonly name: string;
    readonly recordings: ReadonlyArray<{
      readonly id: string;
    }>;
    readonly reservation: {
      readonly assignment: {
        readonly plannedEndAt: string | null | undefined;
        readonly plannedStartAt: string | null | undefined;
        readonly state: AssignmentState;
      };
      readonly enabled: boolean;
      readonly id: string;
      readonly kind: ReservationKind;
      readonly rules: ReadonlyArray<{
        readonly id: string;
        readonly name: string;
      }>;
    } | null | undefined;
    readonly service: {
      readonly id: string;
      readonly logoUrl: string | null | undefined;
      readonly name: string;
      readonly timeshiftPrograms: ReadonlyArray<{
        readonly eventId: number;
        readonly startAt: string;
      }>;
    };
    readonly startAt: string;
    readonly watchReserved: boolean;
  } | {
    // This will never be '%other', but we need some
    // value in case none of the concrete values match.
    readonly __typename: "%other";
  } | null | undefined;
  readonly viewer: {
    readonly role: Role;
  };
};
export type ProgramDetailQuery = {
  response: ProgramDetailQuery$data;
  variables: ProgramDetailQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "id"
  }
],
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "role",
  "storageKey": null
},
v2 = [
  {
    "kind": "Variable",
    "name": "id",
    "variableName": "id"
  }
],
v3 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "__typename",
  "storageKey": null
},
v4 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v5 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v6 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "description",
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
  "name": "endAt",
  "storageKey": null
},
v9 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "endUndecided",
  "storageKey": null
},
v10 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "isFree",
  "storageKey": null
},
v11 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "archived",
  "storageKey": null
},
v12 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "eventId",
  "storageKey": null
},
v13 = {
  "alias": null,
  "args": null,
  "concreteType": "Service",
  "kind": "LinkedField",
  "name": "service",
  "plural": false,
  "selections": [
    (v4/*:: as any*/),
    (v5/*:: as any*/),
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "logoUrl",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "concreteType": "TimeshiftProgram",
      "kind": "LinkedField",
      "name": "timeshiftPrograms",
      "plural": true,
      "selections": [
        (v12/*:: as any*/),
        (v7/*:: as any*/)
      ],
      "storageKey": null
    }
  ],
  "storageKey": null
},
v14 = {
  "alias": null,
  "args": null,
  "concreteType": "Recording",
  "kind": "LinkedField",
  "name": "recordings",
  "plural": true,
  "selections": [
    (v4/*:: as any*/)
  ],
  "storageKey": null
},
v15 = {
  "alias": null,
  "args": null,
  "concreteType": "Genre",
  "kind": "LinkedField",
  "name": "genres",
  "plural": true,
  "selections": [
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
  ],
  "storageKey": null
},
v16 = {
  "alias": null,
  "args": null,
  "concreteType": "AnimeEpisode",
  "kind": "LinkedField",
  "name": "animeEpisodes",
  "plural": true,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "syobocalUrl",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "countStart",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "countEnd",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "subtitle",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "first",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "last",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "concreteType": "AnimeWork",
      "kind": "LinkedField",
      "name": "works",
      "plural": true,
      "selections": [
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "annictUrl",
          "storageKey": null
        },
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "anilistUrl",
          "storageKey": null
        },
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "malUrl",
          "storageKey": null
        }
      ],
      "storageKey": null
    }
  ],
  "storageKey": null
},
v17 = {
  "alias": null,
  "args": null,
  "concreteType": "ExtendedItem",
  "kind": "LinkedField",
  "name": "extended",
  "plural": true,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "heading",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "text",
      "storageKey": null
    }
  ],
  "storageKey": null
},
v18 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "watchReserved",
  "storageKey": null
},
v19 = {
  "alias": null,
  "args": null,
  "concreteType": "Reservation",
  "kind": "LinkedField",
  "name": "reservation",
  "plural": false,
  "selections": [
    (v4/*:: as any*/),
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
      "kind": "ScalarField",
      "name": "kind",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "concreteType": "ReservationAssignment",
      "kind": "LinkedField",
      "name": "assignment",
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
          "name": "plannedStartAt",
          "storageKey": null
        },
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "plannedEndAt",
          "storageKey": null
        }
      ],
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "concreteType": "Rule",
      "kind": "LinkedField",
      "name": "rules",
      "plural": true,
      "selections": [
        (v4/*:: as any*/),
        (v5/*:: as any*/)
      ],
      "storageKey": null
    }
  ],
  "storageKey": null
};
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "ProgramDetailQuery",
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "User",
        "kind": "LinkedField",
        "name": "viewer",
        "plural": false,
        "selections": [
          (v1/*:: as any*/)
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": (v2/*:: as any*/),
        "concreteType": null,
        "kind": "LinkedField",
        "name": "node",
        "plural": false,
        "selections": [
          (v3/*:: as any*/),
          {
            "kind": "InlineFragment",
            "selections": [
              (v4/*:: as any*/),
              (v5/*:: as any*/),
              (v6/*:: as any*/),
              (v7/*:: as any*/),
              (v8/*:: as any*/),
              (v9/*:: as any*/),
              (v10/*:: as any*/),
              (v11/*:: as any*/),
              (v12/*:: as any*/),
              (v13/*:: as any*/),
              (v14/*:: as any*/),
              (v15/*:: as any*/),
              (v16/*:: as any*/),
              (v17/*:: as any*/),
              (v18/*:: as any*/),
              (v19/*:: as any*/)
            ],
            "type": "Program",
            "abstractKey": null
          }
        ],
        "storageKey": null
      }
    ],
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "ProgramDetailQuery",
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "User",
        "kind": "LinkedField",
        "name": "viewer",
        "plural": false,
        "selections": [
          (v1/*:: as any*/),
          (v4/*:: as any*/)
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": (v2/*:: as any*/),
        "concreteType": null,
        "kind": "LinkedField",
        "name": "node",
        "plural": false,
        "selections": [
          (v3/*:: as any*/),
          (v4/*:: as any*/),
          {
            "kind": "InlineFragment",
            "selections": [
              (v5/*:: as any*/),
              (v6/*:: as any*/),
              (v7/*:: as any*/),
              (v8/*:: as any*/),
              (v9/*:: as any*/),
              (v10/*:: as any*/),
              (v11/*:: as any*/),
              (v12/*:: as any*/),
              (v13/*:: as any*/),
              (v14/*:: as any*/),
              (v15/*:: as any*/),
              (v16/*:: as any*/),
              (v17/*:: as any*/),
              (v18/*:: as any*/),
              (v19/*:: as any*/)
            ],
            "type": "Program",
            "abstractKey": null
          }
        ],
        "storageKey": null
      }
    ]
  },
  "params": {
    "cacheID": "2ffcd36c004129fe85abb29344fef7c3",
    "id": null,
    "metadata": {},
    "name": "ProgramDetailQuery",
    "operationKind": "query",
    "text": "query ProgramDetailQuery(\n  $id: ID!\n) {\n  viewer {\n    role\n    id\n  }\n  node(id: $id) {\n    __typename\n    ... on Program {\n      id\n      name\n      description\n      startAt\n      endAt\n      endUndecided\n      isFree\n      archived\n      eventId\n      service {\n        id\n        name\n        logoUrl\n        timeshiftPrograms {\n          eventId\n          startAt\n        }\n      }\n      recordings {\n        id\n      }\n      genres {\n        lv1\n        lv2\n      }\n      animeEpisodes {\n        syobocalUrl\n        countStart\n        countEnd\n        subtitle\n        first\n        last\n        works {\n          annictUrl\n          anilistUrl\n          malUrl\n        }\n      }\n      extended {\n        heading\n        text\n      }\n      watchReserved\n      reservation {\n        id\n        enabled\n        kind\n        assignment {\n          state\n          plannedStartAt\n          plannedEndAt\n        }\n        rules {\n          id\n          name\n        }\n      }\n    }\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "c256983ddb657bf27b86f0010bb79471";

export default node;
