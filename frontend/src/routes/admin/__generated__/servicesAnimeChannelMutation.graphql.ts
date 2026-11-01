/**
 * @generated SignedSource<<da4f3cc056ade4e7eceec6c9a704664f>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type AnimeChannelInput = {
  annictChannelId?: number | null | undefined;
  resetAnnictChannelId?: boolean;
  resetSyobocalChannelId?: boolean;
  syobocalChannelId?: number | null | undefined;
};
export type servicesAnimeChannelMutation$variables = {
  input: AnimeChannelInput;
  serviceId: string;
};
export type servicesAnimeChannelMutation$data = {
  readonly updateAnimeChannel: {
    readonly annictChannelIdOverride: number | null | undefined;
    readonly serviceId: string;
    readonly syobocalChannelIdOverride: number | null | undefined;
  };
};
export type servicesAnimeChannelMutation = {
  response: servicesAnimeChannelMutation$data;
  variables: servicesAnimeChannelMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "input"
},
v1 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "serviceId"
},
v2 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "input",
        "variableName": "input"
      },
      {
        "kind": "Variable",
        "name": "serviceId",
        "variableName": "serviceId"
      }
    ],
    "concreteType": "AnimeChannel",
    "kind": "LinkedField",
    "name": "updateAnimeChannel",
    "plural": false,
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
    "argumentDefinitions": [
      (v0/*:: as any*/),
      (v1/*:: as any*/)
    ],
    "kind": "Fragment",
    "metadata": null,
    "name": "servicesAnimeChannelMutation",
    "selections": (v2/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [
      (v1/*:: as any*/),
      (v0/*:: as any*/)
    ],
    "kind": "Operation",
    "name": "servicesAnimeChannelMutation",
    "selections": (v2/*:: as any*/)
  },
  "params": {
    "cacheID": "43375438ae9041d890467ad3503c2f02",
    "id": null,
    "metadata": {},
    "name": "servicesAnimeChannelMutation",
    "operationKind": "mutation",
    "text": "mutation servicesAnimeChannelMutation(\n  $serviceId: ID!\n  $input: AnimeChannelInput!\n) {\n  updateAnimeChannel(serviceId: $serviceId, input: $input) {\n    serviceId\n    syobocalChannelIdOverride\n    annictChannelIdOverride\n  }\n}\n"
  }
};
})();

(node as any).hash = "e2b3e9d209f02a071de745da3d98884e";

export default node;
