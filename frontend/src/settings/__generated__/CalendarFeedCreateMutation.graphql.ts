/**
 * @generated SignedSource<<79f9efe38dc5684a3601f5e8ab338b3f>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type CalendarFeedCreateMutation$variables = Record<PropertyKey, never>;
export type CalendarFeedCreateMutation$data = {
  readonly createCalendarFeed: {
    readonly path: string;
  };
};
export type CalendarFeedCreateMutation = {
  response: CalendarFeedCreateMutation$data;
  variables: CalendarFeedCreateMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "CreatedCalendarFeed",
    "kind": "LinkedField",
    "name": "createCalendarFeed",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "path",
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
    "name": "CalendarFeedCreateMutation",
    "selections": (v0/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "CalendarFeedCreateMutation",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "96ae01e7f4cfe61a9d301fcad2ef7c32",
    "id": null,
    "metadata": {},
    "name": "CalendarFeedCreateMutation",
    "operationKind": "mutation",
    "text": "mutation CalendarFeedCreateMutation {\n  createCalendarFeed {\n    path\n  }\n}\n"
  }
};
})();

(node as any).hash = "2cd0deb6b5cda17c6e599606a1f8f7d0";

export default node;
