/**
 * @generated SignedSource<<9ab84fcfb2992c778f81c922c7e62e8e>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type channelsQuery$variables = Record<PropertyKey, never>;
export type channelsQuery$data = {
  readonly channelTypes: ReadonlyArray<{
    readonly displayName: string;
    readonly sortOrder: number;
    readonly type: string;
  }>;
  readonly passkeys: ReadonlyArray<{
    readonly id: string;
  }>;
  readonly secondFactors: {
    readonly remainingRecoveryCodes: number;
    readonly totp: boolean;
  };
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
};
export type channelsQuery = {
  response: channelsQuery$data;
  variables: channelsQuery$variables;
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
  "name": "id",
  "storageKey": null
},
v3 = [
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
    "concreteType": "SecondFactorStatus",
    "kind": "LinkedField",
    "name": "secondFactors",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "totp",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "remainingRecoveryCodes",
        "storageKey": null
      }
    ],
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "concreteType": "Passkey",
    "kind": "LinkedField",
    "name": "passkeys",
    "plural": true,
    "selections": [
      (v2/*:: as any*/)
    ],
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "channelsQuery",
    "selections": (v3/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "channelsQuery",
    "selections": (v3/*:: as any*/)
  },
  "params": {
    "cacheID": "a361a95dd54e59d87d267dc6d38730ba",
    "id": null,
    "metadata": {},
    "name": "channelsQuery",
    "operationKind": "query",
    "text": "query channelsQuery {\n  channelTypes {\n    type\n    displayName\n    sortOrder\n  }\n  services(includeHidden: true) {\n    id\n    name\n    logoUrl\n    hidden\n    sortOrder\n    channelType {\n      displayName\n    }\n  }\n  secondFactors {\n    totp\n    remainingRecoveryCodes\n  }\n  passkeys {\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "1bb937f2c4d4c3027d75f4e097c84ad7";

export default node;
