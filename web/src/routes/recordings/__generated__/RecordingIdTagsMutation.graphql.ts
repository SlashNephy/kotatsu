/**
 * @generated SignedSource<<9b93d7cf12614bf0c16a1810c9080b65>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type RecordingIdTagsMutation$variables = {
  id: string;
  tags: ReadonlyArray<string>;
};
export type RecordingIdTagsMutation$data = {
  readonly setRecordingTags: {
    readonly id: string;
    readonly tags: ReadonlyArray<{
      readonly id: string;
      readonly name: string;
    }>;
  };
};
export type RecordingIdTagsMutation = {
  response: RecordingIdTagsMutation$data;
  variables: RecordingIdTagsMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "id"
  },
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "tags"
  }
],
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v2 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "id",
        "variableName": "id"
      },
      {
        "kind": "Variable",
        "name": "tags",
        "variableName": "tags"
      }
    ],
    "concreteType": "Recording",
    "kind": "LinkedField",
    "name": "setRecordingTags",
    "plural": false,
    "selections": [
      (v1/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "concreteType": "Tag",
        "kind": "LinkedField",
        "name": "tags",
        "plural": true,
        "selections": [
          (v1/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "name",
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
    "name": "RecordingIdTagsMutation",
    "selections": (v2/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "RecordingIdTagsMutation",
    "selections": (v2/*:: as any*/)
  },
  "params": {
    "cacheID": "5a2b4b18e7f4e8485fe41f612759683f",
    "id": null,
    "metadata": {},
    "name": "RecordingIdTagsMutation",
    "operationKind": "mutation",
    "text": "mutation RecordingIdTagsMutation(\n  $id: ID!\n  $tags: [String!]!\n) {\n  setRecordingTags(id: $id, tags: $tags) {\n    id\n    tags {\n      id\n      name\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "39d8eff350fb0afb09956679a2d4d943";

export default node;
