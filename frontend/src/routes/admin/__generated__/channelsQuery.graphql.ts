/**
 * @generated SignedSource<<15d82a57332603dcf567e497be6b2138>>
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
  readonly timeshiftAvailable: boolean;
  readonly timeshiftChannels: ReadonlyArray<{
    readonly bufferedFrom: string | null | undefined;
    readonly bufferedUntil: string | null | undefined;
    readonly recording: boolean;
    readonly retentionSeconds: number;
    readonly service: {
      readonly channelType: {
        readonly displayName: string;
      };
      readonly id: string;
      readonly logoUrl: string | null | undefined;
      readonly name: string;
    };
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
v3 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v4 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "logoUrl",
  "storageKey": null
},
v5 = {
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
},
v6 = [
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
      (v3/*:: as any*/),
      (v4/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "hidden",
        "storageKey": null
      },
      (v1/*:: as any*/),
      (v5/*:: as any*/)
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
  },
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "timeshiftAvailable",
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "concreteType": "TimeshiftChannel",
    "kind": "LinkedField",
    "name": "timeshiftChannels",
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
          (v2/*:: as any*/),
          (v3/*:: as any*/),
          (v4/*:: as any*/),
          (v5/*:: as any*/)
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "retentionSeconds",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "bufferedFrom",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "bufferedUntil",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "recording",
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
    "name": "channelsQuery",
    "selections": (v6/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "channelsQuery",
    "selections": (v6/*:: as any*/)
  },
  "params": {
    "cacheID": "57e03cb40954677e365f3e8d36c714e3",
    "id": null,
    "metadata": {},
    "name": "channelsQuery",
    "operationKind": "query",
    "text": "query channelsQuery {\n  channelTypes {\n    type\n    displayName\n    sortOrder\n  }\n  services(includeHidden: true) {\n    id\n    name\n    logoUrl\n    hidden\n    sortOrder\n    channelType {\n      displayName\n    }\n  }\n  secondFactors {\n    totp\n    remainingRecoveryCodes\n  }\n  passkeys {\n    id\n  }\n  timeshiftAvailable\n  timeshiftChannels {\n    service {\n      id\n      name\n      logoUrl\n      channelType {\n        displayName\n      }\n    }\n    retentionSeconds\n    bufferedFrom\n    bufferedUntil\n    recording\n  }\n}\n"
  }
};
})();

(node as any).hash = "b3f3371a2dfdc270b1ff1a2ed927d8f0";

export default node;
