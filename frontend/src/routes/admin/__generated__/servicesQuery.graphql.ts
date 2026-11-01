/**
 * @generated SignedSource<<6445cd1787a1212c40f2103b23e0f592>>
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
    readonly hidden: boolean;
    readonly id: string;
    readonly logoUrl: string | null | undefined;
    readonly name: string;
    readonly sortOrder: number | null | undefined;
  }>;
  readonly systemSettings: {
    readonly animeLinkingEnabled: boolean;
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
v2 = [
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
    "storageKey": "services(includeHidden:true)"
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
    "selections": (v2/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "servicesQuery",
    "selections": (v2/*:: as any*/)
  },
  "params": {
    "cacheID": "9e61484e85d8701692092215cc1ef6bd",
    "id": null,
    "metadata": {},
    "name": "servicesQuery",
    "operationKind": "query",
    "text": "query servicesQuery {\n  channelTypes {\n    type\n    displayName\n    sortOrder\n  }\n  services(includeHidden: true) {\n    id\n    name\n    logoUrl\n    hidden\n    sortOrder\n    channelType {\n      displayName\n    }\n  }\n  systemSettings {\n    animeLinkingEnabled\n  }\n  animeChannels {\n    serviceId\n    definitionName\n    importedSyobocalChannelId\n    importedAnnictChannelId\n    syobocalChannelIdOverride\n    annictChannelIdOverride\n  }\n}\n"
  }
};
})();

(node as any).hash = "628efdd82cf5918a2218cbc189c474d1";

export default node;
