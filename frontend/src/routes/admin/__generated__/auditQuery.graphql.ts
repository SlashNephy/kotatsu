/**
 * @generated SignedSource<<c8c9f214fc360fee77f7795e7810b6be>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type AuditEventFilter = {
  actionPrefix?: string | null | undefined;
  actorUserId?: string | null | undefined;
  since?: string | null | undefined;
  targetId?: string | null | undefined;
  targetType?: string | null | undefined;
  until?: string | null | undefined;
};
export type auditQuery$variables = {
  after?: string | null | undefined;
  filter?: AuditEventFilter | null | undefined;
  first: number;
};
export type auditQuery$data = {
  readonly auditEvents: {
    readonly edges: ReadonlyArray<{
      readonly node: {
        readonly action: string;
        readonly actor: {
          readonly role: string | null | undefined;
          readonly type: string;
          readonly user: {
            readonly displayName: string;
            readonly username: string;
          } | null | undefined;
        };
        readonly id: string;
        readonly occurredAt: string;
        readonly sourceIp: string | null | undefined;
        readonly summary: string;
        readonly targetId: string | null | undefined;
        readonly targetType: string;
        readonly userAgent: string | null | undefined;
      };
    }>;
    readonly pageInfo: {
      readonly endCursor: string | null | undefined;
      readonly hasNextPage: boolean;
    };
  };
  readonly users: ReadonlyArray<{
    readonly displayName: string;
    readonly id: string;
    readonly username: string;
  }>;
};
export type auditQuery = {
  response: auditQuery$data;
  variables: auditQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "after"
},
v1 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "filter"
},
v2 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "first"
},
v3 = [
  {
    "kind": "Variable",
    "name": "after",
    "variableName": "after"
  },
  {
    "kind": "Variable",
    "name": "filter",
    "variableName": "filter"
  },
  {
    "kind": "Variable",
    "name": "first",
    "variableName": "first"
  }
],
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
  "name": "occurredAt",
  "storageKey": null
},
v6 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "type",
  "storageKey": null
},
v7 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "username",
  "storageKey": null
},
v8 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "displayName",
  "storageKey": null
},
v9 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "role",
  "storageKey": null
},
v10 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "action",
  "storageKey": null
},
v11 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "targetType",
  "storageKey": null
},
v12 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "targetId",
  "storageKey": null
},
v13 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "summary",
  "storageKey": null
},
v14 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "sourceIp",
  "storageKey": null
},
v15 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "userAgent",
  "storageKey": null
},
v16 = {
  "alias": null,
  "args": null,
  "concreteType": "PageInfo",
  "kind": "LinkedField",
  "name": "pageInfo",
  "plural": false,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "hasNextPage",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "endCursor",
      "storageKey": null
    }
  ],
  "storageKey": null
},
v17 = {
  "alias": null,
  "args": null,
  "concreteType": "User",
  "kind": "LinkedField",
  "name": "users",
  "plural": true,
  "selections": [
    (v4/*:: as any*/),
    (v7/*:: as any*/),
    (v8/*:: as any*/)
  ],
  "storageKey": null
};
return {
  "fragment": {
    "argumentDefinitions": [
      (v0/*:: as any*/),
      (v1/*:: as any*/),
      (v2/*:: as any*/)
    ],
    "kind": "Fragment",
    "metadata": null,
    "name": "auditQuery",
    "selections": [
      {
        "alias": null,
        "args": (v3/*:: as any*/),
        "concreteType": "AuditEventConnection",
        "kind": "LinkedField",
        "name": "auditEvents",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "concreteType": "AuditEventEdge",
            "kind": "LinkedField",
            "name": "edges",
            "plural": true,
            "selections": [
              {
                "alias": null,
                "args": null,
                "concreteType": "AuditEvent",
                "kind": "LinkedField",
                "name": "node",
                "plural": false,
                "selections": [
                  (v4/*:: as any*/),
                  (v5/*:: as any*/),
                  {
                    "alias": null,
                    "args": null,
                    "concreteType": "AuditActor",
                    "kind": "LinkedField",
                    "name": "actor",
                    "plural": false,
                    "selections": [
                      (v6/*:: as any*/),
                      {
                        "alias": null,
                        "args": null,
                        "concreteType": "User",
                        "kind": "LinkedField",
                        "name": "user",
                        "plural": false,
                        "selections": [
                          (v7/*:: as any*/),
                          (v8/*:: as any*/)
                        ],
                        "storageKey": null
                      },
                      (v9/*:: as any*/)
                    ],
                    "storageKey": null
                  },
                  (v10/*:: as any*/),
                  (v11/*:: as any*/),
                  (v12/*:: as any*/),
                  (v13/*:: as any*/),
                  (v14/*:: as any*/),
                  (v15/*:: as any*/)
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          },
          (v16/*:: as any*/)
        ],
        "storageKey": null
      },
      (v17/*:: as any*/)
    ],
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [
      (v2/*:: as any*/),
      (v0/*:: as any*/),
      (v1/*:: as any*/)
    ],
    "kind": "Operation",
    "name": "auditQuery",
    "selections": [
      {
        "alias": null,
        "args": (v3/*:: as any*/),
        "concreteType": "AuditEventConnection",
        "kind": "LinkedField",
        "name": "auditEvents",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "concreteType": "AuditEventEdge",
            "kind": "LinkedField",
            "name": "edges",
            "plural": true,
            "selections": [
              {
                "alias": null,
                "args": null,
                "concreteType": "AuditEvent",
                "kind": "LinkedField",
                "name": "node",
                "plural": false,
                "selections": [
                  (v4/*:: as any*/),
                  (v5/*:: as any*/),
                  {
                    "alias": null,
                    "args": null,
                    "concreteType": "AuditActor",
                    "kind": "LinkedField",
                    "name": "actor",
                    "plural": false,
                    "selections": [
                      (v6/*:: as any*/),
                      {
                        "alias": null,
                        "args": null,
                        "concreteType": "User",
                        "kind": "LinkedField",
                        "name": "user",
                        "plural": false,
                        "selections": [
                          (v7/*:: as any*/),
                          (v8/*:: as any*/),
                          (v4/*:: as any*/)
                        ],
                        "storageKey": null
                      },
                      (v9/*:: as any*/)
                    ],
                    "storageKey": null
                  },
                  (v10/*:: as any*/),
                  (v11/*:: as any*/),
                  (v12/*:: as any*/),
                  (v13/*:: as any*/),
                  (v14/*:: as any*/),
                  (v15/*:: as any*/)
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          },
          (v16/*:: as any*/)
        ],
        "storageKey": null
      },
      (v17/*:: as any*/)
    ]
  },
  "params": {
    "cacheID": "e712290d226b8e03e87760b2e04d8ecc",
    "id": null,
    "metadata": {},
    "name": "auditQuery",
    "operationKind": "query",
    "text": "query auditQuery(\n  $first: Int!\n  $after: String\n  $filter: AuditEventFilter\n) {\n  auditEvents(first: $first, after: $after, filter: $filter) {\n    edges {\n      node {\n        id\n        occurredAt\n        actor {\n          type\n          user {\n            username\n            displayName\n            id\n          }\n          role\n        }\n        action\n        targetType\n        targetId\n        summary\n        sourceIp\n        userAgent\n      }\n    }\n    pageInfo {\n      hasNextPage\n      endCursor\n    }\n  }\n  users {\n    id\n    username\n    displayName\n  }\n}\n"
  }
};
})();

(node as any).hash = "f85f71013330c2e8bba63b69887d0653";

export default node;
