/**
 * @generated SignedSource<<ac5cf8ccfad8d7815f5a14dd94c15bc8>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type servicesQuery$variables = Record<PropertyKey, never>;
export type servicesQuery$data = {
  readonly animeChannels: ReadonlyArray<{
    readonly annictChannelIdOverride: number | null | undefined;
    readonly definitionName: string | null | undefined;
    readonly importedAnnictChannelId: number | null | undefined;
    readonly importedSyobocalChannelId: number | null | undefined;
    readonly serviceId: string;
    readonly syobocalChannelIdOverride: number | null | undefined;
  }>;
  readonly channelTypes: ReadonlyArray<{
    readonly displayName: string;
    readonly sortOrder: number;
    readonly type: string;
  }>;
  readonly services: ReadonlyArray<{
    readonly channelType: {
      readonly displayName: string;
    };
    readonly gone: boolean;
    readonly hidden: boolean;
    readonly id: string;
    readonly logoUrl: string | null | undefined;
    readonly name: string;
    readonly networkId: number;
    readonly serviceId: number;
    readonly sortOrder: number | null | undefined;
  }>;
  readonly systemSettings: {
    readonly animeLinkingEnabled: boolean;
    readonly jikkyoChannels: ReadonlyArray<{
      readonly channelId: string;
      readonly networkId: number;
      readonly serviceId: number;
    }>;
  };
};
export type servicesQuery = {
  response: servicesQuery$data;
  variables: servicesQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "displayName",
  "storageKey": null
},
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "sortOrder",
  "storageKey": null
},
v2 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "networkId",
  "storageKey": null
},
v3 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "serviceId",
  "storageKey": null
},
v4 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "ChannelType",
    "kind": "LinkedField",
    "name": "channelTypes",
    "plural": true,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "type",
        "storageKey": null
      },
      (v0/*:: as any*/),
      (v1/*:: as any*/)
    ],
    "storageKey": null
  },
  {
    "alias": null,
    "args": [
      {
        "kind": "Literal",
        "name": "includeGone",
        "value": true
      },
      {
        "kind": "Literal",
        "name": "includeHidden",
        "value": true
      }
    ],
    "concreteType": "Service",
    "kind": "LinkedField",
    "name": "services",
    "plural": true,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "id",
        "storageKey": null
      },
      (v2/*:: as any*/),
      (v3/*:: as any*/),
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
        "name": "logoUrl",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "hidden",
        "storageKey": null
      },
      (v1/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "gone",
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
          (v0/*:: as any*/)
        ],
        "storageKey": null
      }
    ],
    "storageKey": "services(includeGone:true,includeHidden:true)"
  },
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
        "kind": "ScalarField",
        "name": "animeLinkingEnabled",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "JikkyoChannelOverride",
        "kind": "LinkedField",
        "name": "jikkyoChannels",
        "plural": true,
        "selections": [
          (v2/*:: as any*/),
          (v3/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "channelId",
            "storageKey": null
          }
        ],
        "storageKey": null
      }
    ],
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "concreteType": "AnimeChannel",
    "kind": "LinkedField",
    "name": "animeChannels",
    "plural": true,
    "selections": [
      (v3/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "definitionName",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "importedSyobocalChannelId",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "importedAnnictChannelId",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "syobocalChannelIdOverride",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "annictChannelIdOverride",
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
    "name": "servicesQuery",
    "selections": (v4/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "servicesQuery",
    "selections": (v4/*:: as any*/)
  },
  "params": {
    "cacheID": "5a93040fd400d1700b26190eb131fcc8",
    "id": null,
    "metadata": {},
    "name": "servicesQuery",
    "operationKind": "query",
    "text": "query servicesQuery {\n  channelTypes {\n    type\n    displayName\n    sortOrder\n  }\n  services(includeHidden: true, includeGone: true) {\n    id\n    networkId\n    serviceId\n    name\n    logoUrl\n    hidden\n    sortOrder\n    gone\n    channelType {\n      displayName\n    }\n  }\n  systemSettings {\n    animeLinkingEnabled\n    jikkyoChannels {\n      networkId\n      serviceId\n      channelId\n    }\n  }\n  animeChannels {\n    serviceId\n    definitionName\n    importedSyobocalChannelId\n    importedAnnictChannelId\n    syobocalChannelIdOverride\n    annictChannelIdOverride\n  }\n}\n"
  }
};
})();

(node as any).hash = "f7966671df37f94a56901f846c0c5ad4";

export default node;
