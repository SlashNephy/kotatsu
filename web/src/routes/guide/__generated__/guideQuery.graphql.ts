/**
 * @generated SignedSource<<487d280b29ecaeba859953f326ca0a49>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type guideQuery$variables = {
  from: string;
  to: string;
  view?: string | null | undefined;
};
export type guideQuery$data = {
  readonly channelTypes: ReadonlyArray<{
    readonly displayName: string;
    readonly type: string;
  }>;
  readonly guide: ReadonlyArray<{
    readonly programs: ReadonlyArray<{
      readonly endAt: string;
      readonly endUndecided: boolean;
      readonly genres: ReadonlyArray<{
        readonly lv1: number;
      }>;
      readonly id: string;
      readonly name: string;
      readonly reservation: {
        readonly enabled: boolean;
        readonly id: string;
      } | null | undefined;
      readonly sharedFrom: {
        readonly id: string;
      } | null | undefined;
      readonly startAt: string;
      readonly watchReserved: boolean;
    }>;
    readonly service: {
      readonly channelType: {
        readonly type: string;
      };
      readonly id: string;
      readonly logoUrl: string | null | undefined;
      readonly name: string;
    };
  }>;
  readonly guideViews: ReadonlyArray<{
    readonly excludeGenres: ReadonlyArray<{
      readonly lv1: number;
      readonly lv2: number | null | undefined;
    }>;
    readonly freeOnly: boolean;
    readonly genres: ReadonlyArray<{
      readonly lv1: number;
      readonly lv2: number | null | undefined;
    }>;
    readonly hideEmptyServices: boolean;
    readonly id: string;
    readonly minDurationSeconds: number | null | undefined;
    readonly name: string;
    readonly services: ReadonlyArray<{
      readonly id: string;
    }>;
  }>;
  readonly pinnedServices: ReadonlyArray<{
    readonly id: string;
  }>;
  readonly services: ReadonlyArray<{
    readonly channelType: {
      readonly displayName: string;
    };
    readonly id: string;
    readonly logoUrl: string | null | undefined;
    readonly name: string;
    readonly serviceId: number;
  }>;
};
export type guideQuery = {
  response: guideQuery$data;
  variables: guideQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "from"
  },
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "to"
  },
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "view"
  }
],
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "type",
  "storageKey": null
},
v2 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "displayName",
  "storageKey": null
},
v3 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v4 = [
  (v3/*:: as any*/)
],
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
  "name": "logoUrl",
  "storageKey": null
},
v7 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "lv1",
  "storageKey": null
},
v8 = [
  (v7/*:: as any*/),
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "lv2",
    "storageKey": null
  }
],
v9 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "ChannelType",
    "kind": "LinkedField",
    "name": "channelTypes",
    "plural": true,
    "selections": [
      (v1/*:: as any*/),
      (v2/*:: as any*/)
    ],
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "concreteType": "Service",
    "kind": "LinkedField",
    "name": "pinnedServices",
    "plural": true,
    "selections": (v4/*:: as any*/),
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "concreteType": "Service",
    "kind": "LinkedField",
    "name": "services",
    "plural": true,
    "selections": [
      (v3/*:: as any*/),
      (v5/*:: as any*/),
      (v6/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "serviceId",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "ChannelType",
        "kind": "LinkedField",
        "name": "channelType",
        "plural": false,
        "selections": [
          (v2/*:: as any*/)
        ],
        "storageKey": null
      }
    ],
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "concreteType": "GuideView",
    "kind": "LinkedField",
    "name": "guideViews",
    "plural": true,
    "selections": [
      (v3/*:: as any*/),
      (v5/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "concreteType": "GenreCondition",
        "kind": "LinkedField",
        "name": "genres",
        "plural": true,
        "selections": (v8/*:: as any*/),
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "GenreCondition",
        "kind": "LinkedField",
        "name": "excludeGenres",
        "plural": true,
        "selections": (v8/*:: as any*/),
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
        "kind": "ScalarField",
        "name": "hideEmptyServices",
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
    "args": [
      {
        "kind": "Variable",
        "name": "from",
        "variableName": "from"
      },
      {
        "kind": "Variable",
        "name": "to",
        "variableName": "to"
      },
      {
        "kind": "Variable",
        "name": "view",
        "variableName": "view"
      }
    ],
    "concreteType": "GuideRow",
    "kind": "LinkedField",
    "name": "guide",
    "plural": true,
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "Service",
        "kind": "LinkedField",
        "name": "service",
        "plural": false,
        "selections": [
          (v3/*:: as any*/),
          (v5/*:: as any*/),
          (v6/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "concreteType": "ChannelType",
            "kind": "LinkedField",
            "name": "channelType",
            "plural": false,
            "selections": [
              (v1/*:: as any*/)
            ],
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "Program",
        "kind": "LinkedField",
        "name": "programs",
        "plural": true,
        "selections": [
          (v3/*:: as any*/),
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
            "name": "endAt",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "endUndecided",
            "storageKey": null
          },
          (v5/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "concreteType": "Genre",
            "kind": "LinkedField",
            "name": "genres",
            "plural": true,
            "selections": [
              (v7/*:: as any*/)
            ],
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "concreteType": "Service",
            "kind": "LinkedField",
            "name": "sharedFrom",
            "plural": false,
            "selections": (v4/*:: as any*/),
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "concreteType": "Reservation",
            "kind": "LinkedField",
            "name": "reservation",
            "plural": false,
            "selections": [
              (v3/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "enabled",
                "storageKey": null
              }
            ],
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "watchReserved",
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
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "guideQuery",
    "selections": (v9/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "guideQuery",
    "selections": (v9/*:: as any*/)
  },
  "params": {
    "cacheID": "6b4688a88ff08302bc7a5e0e6387e66d",
    "id": null,
    "metadata": {},
    "name": "guideQuery",
    "operationKind": "query",
    "text": "query guideQuery(\n  $from: Time!\n  $to: Time!\n  $view: ID\n) {\n  channelTypes {\n    type\n    displayName\n  }\n  pinnedServices {\n    id\n  }\n  services {\n    id\n    name\n    logoUrl\n    serviceId\n    channelType {\n      displayName\n    }\n  }\n  guideViews {\n    id\n    name\n    genres {\n      lv1\n      lv2\n    }\n    excludeGenres {\n      lv1\n      lv2\n    }\n    minDurationSeconds\n    services {\n      id\n    }\n    hideEmptyServices\n    freeOnly\n  }\n  guide(from: $from, to: $to, view: $view) {\n    service {\n      id\n      name\n      logoUrl\n      channelType {\n        type\n      }\n    }\n    programs {\n      id\n      startAt\n      endAt\n      endUndecided\n      name\n      genres {\n        lv1\n      }\n      sharedFrom {\n        id\n      }\n      reservation {\n        id\n        enabled\n      }\n      watchReserved\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "4a70b4e63f5c4c6ea62f7ec47bb169cc";

export default node;
